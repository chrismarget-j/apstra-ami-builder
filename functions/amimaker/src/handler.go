package amimaker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	cloudinit "github.com/chrismarget-j/apstra-ami-builder/functions/cloudinit/src"
	"log"
	"math/rand"
	"os"
	"path"
	"time"
)

const (
	expectedEvent  = "copySnapshot"
	expectedResult = "succeeded"

	msgIgnoreEvent = "ignoring un-interesting event"
	msgSuccess     = "clean exit AMI is '%s'"

	cloudInitTagKey = "cloud-init"
	truthyString    = "true"

	envVarInstallCiLambdaName        = "INSTALL_CI_LAMBDA_NAME"
	envVarInstallCiLambdaSG          = "INSTALL_CI_LAMBDA_SECURITY_GROUP"
	envVarInstanceType               = "INSTANCE_TYPE"
	envVarRetainIntermediateSnapshot = "KEEP_INTERMEDIATE_SNAPSHOT"
	envVarRetainIntermediateAmi      = "KEEP_INTERMEDIATE_AMI"
	envVarRetainIntermediateInstance = "KEEP_INTERMEDIATE_INSTANCE"

	ec2InstanceBootWaitInterval = 45 * time.Second
	apiIterationWait            = 250 * time.Millisecond

	dyingBreathInterval = 5 * time.Second
)

type Request struct {
	Account string `json:"account"`
	Detail  struct {
		Cause       string    `json:"cause"`
		EndTime     time.Time `json:"endTime"`
		Event       string    `json:"event"`
		Incremental string    `json:"incremental"`
		RequestId   string    `json:"request-id"`
		Result      string    `json:"result"`
		SnapshotId  string    `json:"snapshot_id"`
		Source      string    `json:"source"`
		StartTime   time.Time `json:"startTime"`
	} `json:"detail"`
	DetailType string    `json:"detail-type"`
	Id         string    `json:"id"`
	Region     string    `json:"region"`
	Resources  []string  `json:"resources"`
	Source     string    `json:"source"`
	Time       time.Time `json:"time"`
	Version    string    `json:"version"`
}

type Response struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	AmiId   string `json:"ami_id,omitempty"`
}

func HandleRequest(ctx context.Context, request *Request) (*Response, error) {
	reqDump, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling request - %w", err)
	}
	log.Printf("request received: '%s'", string(reqDump))

	if deadline, ok := ctx.Deadline(); ok {
		fmt.Printf("context deadline found: '%s' (%s)\n", deadline, time.Now().Sub(deadline))
	} else {
		fmt.Println("no context deadline")
	}

	h, err := newHandler(ctx, request)
	if err != nil {
		err = fmt.Errorf("error creating new handler - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.filterEvents()
	if err != nil {
		var hErr handlerErr
		if errors.As(err, &hErr) && hErr.Type() == errFilterFail {
			log.Printf("filter says nope to this event")
			return &Response{Message: msgIgnoreEvent}, nil
		}
		log.Printf("some other error: '%s'", err.Error())
		err = fmt.Errorf("error filtering incoming event - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.testNextStage(ctx)
	if err != nil {
		err = fmt.Errorf("error testing next lambda - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.findImportTask(ctx)
	if err != nil {
		err = fmt.Errorf("error finding import task - %w", err)
		return &Response{
			Error: err.Error(),
		}, err
	}
	if h.task == nil {
		msg := fmt.Sprintf("task which imported snapshot '%s' not found among '%s' events",
			h.request.Detail.SnapshotId,
			expectedEvent)
		log.Print(msg)
		return &Response{Message: msg}, nil
	}
	log.Printf("snapshot originated from task '%s'", *h.task.ImportTaskId)

	err = h.tagSnapshot(ctx)
	if err != nil {
		err = fmt.Errorf("error tagging snapshot - %w", err)
		return &Response{Error: err.Error()}, err
	}
	dump, _ := json.Marshal(h.tags)
	log.Printf("snapshot '%s' tagged: '%s'", h.snapshotId, string(dump))

	err = h.makeTempAmi(ctx)
	if err != nil {
		err = fmt.Errorf("error creating temporary AMI - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("temporary AMI '%s' created", h.tempAmiId)

	err = h.boot(ctx)
	if err != nil {
		err = fmt.Errorf("error starting temporary instance from AMI - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("launched instance '%s', waiting for private IP to appear...", h.tempInstanceId)

	var cancel context.CancelFunc
	var instanceCtx context.Context
	if deadline, ok := ctx.Deadline(); ok {
		instanceCtx, cancel = context.WithDeadline(ctx, deadline.Add(-dyingBreathInterval))
		defer cancel()
		h.deathbedTerminateInstance(ctx)
	} else {
		instanceCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	// todo delete these
	d, _ := instanceCtx.Deadline()
	fmt.Printf("context deadline is now: '%s' (%s)\n", d, time.Now().Sub(d))

	ip, err := h.waitPrivateIp(instanceCtx)
	if err != nil {
		return h.terminateInstance(ctx, err.Error(), true)
	}
	log.Printf("private ip: '%s'", *ip)

	log.Printf("invoking '%s' in %s", h.installCiLambdaName, ec2InstanceBootWaitInterval)
	time.Sleep(ec2InstanceBootWaitInterval)

	log.Printf("invoking '%s' now", h.installCiLambdaName)
	err = h.invokeNextStage(instanceCtx, ip)
	if err != nil {
		return h.terminateInstance(ctx, err.Error(), true)
	}

	err = h.waitUntilStopped(instanceCtx)
	if err != nil {
		return h.terminateInstance(ctx, err.Error(), true)
	}
	log.Printf("instance '%s' stopped.", h.tempInstanceId)
	cancel()

	h.setCiTagTrue()

	amiId, err := h.makeFinalAmi(ctx)
	if err != nil {
		err = fmt.Errorf("error while making final AMI - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.cleanup(ctx)
	if err != nil {
		err = fmt.Errorf("error in cleanup - %w", err)
		return &Response{Error: err.Error()}, err
	}

	return &Response{Message: msgSuccess, AmiId: *amiId}, nil
}

func newHandler(ctx context.Context, request *Request) (*handler, error) {
	installCiLambda, ok := os.LookupEnv(envVarInstallCiLambdaName)
	if !ok {
		return nil, fmt.Errorf("error '%s' unset", envVarInstallCiLambdaName)
	}

	securityGroup, ok := os.LookupEnv(envVarInstallCiLambdaSG)
	if !ok {
		return nil, fmt.Errorf("error '%s' unset", envVarInstallCiLambdaSG)
	}

	instanceType, ok := os.LookupEnv(envVarInstanceType)
	if !ok {
		return nil, fmt.Errorf("error '%s' unset", envVarInstanceType)
	}

	if request == nil {
		return nil, errors.New("nil request")
	}

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading default AWS config - %w", err)
	}

	snapshotArn, err := arn.Parse(request.Detail.SnapshotId)
	if err != nil {
		return nil, fmt.Errorf("error parsing snapshot arn ('%s') found in request - %w", request.Detail.SnapshotId, err)
	}

	return &handler{
		ec2Client:            ec2.NewFromConfig(awsCfg),
		lambdaClient:         lambda.NewFromConfig(awsCfg),
		request:              request,
		snapshotId:           path.Base(snapshotArn.Resource),
		installCiSecurityGrp: securityGroup,
		installCiLambdaName:  installCiLambda,
		instanceType:         instanceType,
		keepAmi:              os.Getenv(envVarRetainIntermediateAmi) == truthyString,
		keepSnapshot:         os.Getenv(envVarRetainIntermediateSnapshot) == truthyString,
		keepInstance:         os.Getenv(envVarRetainIntermediateInstance) == truthyString,
	}, nil
}

type handler struct {
	request              *Request
	ec2Client            *ec2.Client
	lambdaClient         *lambda.Client
	task                 *types.ImportSnapshotTask
	snapshotId           string
	tempAmiId            string
	tempInstanceId       string
	name                 string
	instanceType         string
	installCiLambdaName  string
	installCiSecurityGrp string
	keepSnapshot         bool
	keepAmi              bool
	keepInstance         bool
	tags                 []types.Tag
}

func (o *handler) filterEvents() error {
	if o.request.Detail.Event != expectedEvent {
		log.Printf("expected event '%s', got '%s'", expectedEvent, o.request.Detail.Event)
		return handlerErr{
			errType: errFilterFail,
			err:     fmt.Errorf("expected event '%s', got '%s'", expectedEvent, o.request.Detail.Event),
		}
	}

	if o.request.Detail.Result != expectedResult {
		log.Printf("expected result '%s', got '%s'", expectedResult, o.request.Detail.Result)
		return handlerErr{
			errType: errFilterFail,
			err:     fmt.Errorf("expected result '%s', got '%s'", expectedResult, o.request.Detail.Result),
		}
	}

	return nil
}

func (o *handler) findImportTask(ctx context.Context) error {
	paginator := ec2.NewDescribeImportSnapshotTasksPaginator(o.ec2Client, nil)

pageLoop:
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("error getting task descriptions from paginator - %w", err)
		}
	taskLoop:
		for _, task := range page.ImportSnapshotTasks {
			if task.SnapshotTaskDetail == nil || task.SnapshotTaskDetail.SnapshotId == nil {
				continue taskLoop
			}
			if *task.SnapshotTaskDetail.SnapshotId == o.snapshotId {
				o.task = &task
				o.tags = task.Tags
				break pageLoop
			}
		}
	}
	return nil
}

func (o *handler) tagSnapshot(ctx context.Context) error {
	if o.task == nil {
		return errors.New("handler task is nil while looking for tags")
	}

	_, err := o.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{o.snapshotId},
		Tags:      o.tags,
	})
	if err != nil {
		return fmt.Errorf("error copying tags from snapshot import task to snapshot - %w", err)
	}

	return nil
}

func (o *handler) makeTempAmi(ctx context.Context) error {
	if o == nil || o.task == nil {
		return errors.New("nil values at entry to makeTempAmi")
	}

	rii := &ec2.RegisterImageInput{
		Name:         aws.String(nameFromTagsOrRandom(o.tags) + "-" + randString(6)),
		Architecture: types.ArchitectureValues(types.ArchitectureTypeX8664),
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sda1"),
			Ebs: &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				SnapshotId:          aws.String(o.snapshotId),
				VolumeType:          types.VolumeTypeGp2,
			},
		}},
		Description:        aws.String("scratchpad AMI for cloud-init installation"),
		EnaSupport:         aws.Bool(true),
		ImdsSupport:        types.ImdsSupportValuesV20,
		RootDeviceName:     aws.String("/dev/sda1"),
		VirtualizationType: aws.String(string(types.VirtualizationTypeHvm)),
	}

	rio, err := o.ec2Client.RegisterImage(ctx, rii)
	if err != nil {
		return fmt.Errorf("error importing snapshot - %w", err)
	}
	if rio == nil {
		return errors.New("nil return from registerImage")
	}
	if rio.ImageId == nil {
		return errors.New("nil ImageId return from registerImage")
	}

	o.tempAmiId = *rio.ImageId

	_, err = o.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{path.Base(o.tempAmiId)},
		Tags:      o.tags,
	})

	return nil
}

func (o *handler) boot(ctx context.Context) error {
	rii := &ec2.RunInstancesInput{
		MaxCount:                          aws.Int32(1),
		MinCount:                          aws.Int32(1),
		ImageId:                           aws.String(o.tempAmiId),
		InstanceInitiatedShutdownBehavior: types.ShutdownBehaviorStop,
		InstanceType:                      types.InstanceType(o.instanceType),
		SecurityGroupIds:                  []string{o.installCiSecurityGrp},
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeInstance,
			Tags:         o.tags,
		}},
	}
	rio, err := o.ec2Client.RunInstances(ctx, rii)
	if err != nil {
		return fmt.Errorf("error running temporary EC2 instance for cloud-init installation - %w", err)
	}
	if len(rio.Instances) != 1 {
		dump, _ := json.Marshal(rio)
		return fmt.Errorf("expected to start 1 instance, got %d instances - full output - %s", len(rio.Instances), string(dump))
	}
	o.tempInstanceId = *rio.Instances[0].InstanceId

	return nil
}

func (o *handler) getPrivateIp(ctx context.Context, id string) (*string, error) {
	instance, err := o.getInstance(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error getting instance while determining private IP - %w", err)
	}
	return instance.PrivateIpAddress, nil
}

func (o *handler) waitPrivateIp(ctx context.Context) (*string, error) {
	for {
		ip, err := o.getPrivateIp(ctx, o.tempInstanceId)
		if err != nil {
			return nil, fmt.Errorf("error while waiting for private IP - %w", err)
		}
		if ip != nil {
			return ip, nil
		}
		time.Sleep(apiIterationWait)
	}
}

func (o *handler) waitUntilStopped(ctx context.Context) error {
	log.Printf("waiting for instance '%s' to stop...", o.tempInstanceId)
	for {
		instance, err := o.getInstance(ctx, o.tempInstanceId)
		if err != nil {
			return fmt.Errorf("error getting instance state while waiting for it to stop")
		}
		if instance.State.Name == types.InstanceStateNameStopped {
			return nil
		}
		time.Sleep(apiIterationWait)
	}
}

func (o *handler) getInstance(ctx context.Context, id string) (*types.Instance, error) {
	params := &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	}
	paginator := ec2.NewDescribeInstancesPaginator(o.ec2Client, params)
	for paginator.HasMorePages() {
		var err error
		var dio *ec2.DescribeInstancesOutput
		for {
			dio, err = paginator.NextPage(ctx)
			if err == nil {
				break
			}
			time.Sleep(apiIterationWait)
		}
		if err != nil {
			return nil, fmt.Errorf("error getting instance descriptions from paginator - %w", err)
		}
		return &dio.Reservations[0].Instances[0], nil
	}
	return nil, fmt.Errorf("lone instance not found in paginator")
}

func (o *handler) testNextStage(ctx context.Context) error {
	payload, err := json.Marshal(&cloudinit.Request{Operation: cloudinit.MessagePing})
	if err != nil {
		return fmt.Errorf("error marshaling next lambda's request - %w", err)
	}

	ii := &lambda.InvokeInput{
		FunctionName: aws.String(o.installCiLambdaName),
		Payload:      payload,
	}
	io, err := o.lambdaClient.Invoke(ctx, ii)
	if err != nil {
		return fmt.Errorf("error invoking '%s' lambda - %w", o.installCiLambdaName, err)
	}

	ciResponse := &cloudinit.Response{}
	err = json.Unmarshal(io.Payload, ciResponse)
	if err != nil {
		return fmt.Errorf("error unmarshaling next lambda's reply - %w", err)
	}
	if ciResponse.Error != "" {
		return fmt.Errorf("next lambda returned an error - %s", ciResponse.Error)
	}
	if ciResponse.Message != cloudinit.MessagePong {
		return fmt.Errorf("next lambda didn't say '%s'", cloudinit.MessagePong)
	}

	log.Printf("next lambda is ready to go!")
	return nil
}

func (o *handler) invokeNextStage(ctx context.Context, ip *string) error {
	payload, err := json.Marshal(&cloudinit.Request{
		Operation:  cloudinit.OperationInstall,
		InstanceIp: *ip,
	})
	if err != nil {
		return fmt.Errorf("error marshaling next lambda's request - %w", err)
	}

	ii := &lambda.InvokeInput{
		FunctionName: aws.String(o.installCiLambdaName),
		Payload:      payload,
	}
	io, err := o.lambdaClient.Invoke(ctx, ii)
	if err != nil {
		return fmt.Errorf("error invoking '%s' lambda - %w", o.installCiLambdaName, err)
	}

	ciResponse := &cloudinit.Response{}
	err = json.Unmarshal(io.Payload, ciResponse)
	if err != nil {
		return fmt.Errorf("error unmarshaling reply from '%s' - %w", o.installCiLambdaName, err)
	}
	if ciResponse.Error != "" {
		return fmt.Errorf("'%s' returned an error - %s", o.installCiLambdaName, ciResponse.Error)
	}
	if ciResponse.Message != cloudinit.MessageSuccess {
		return fmt.Errorf("unexpected reply from '%s' - %s", o.installCiLambdaName, string(io.Payload))
	}

	log.Printf("'%s' said: '%s'", o.installCiLambdaName, string(io.Payload))
	return nil
}

func (o *handler) deathbedTerminateInstance(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err == context.Canceled {
				return
			}

			log.Printf("terminating instance '%s' while on our deathbed because '%s'", o.tempInstanceId, err.Error())
			_, err = o.terminateInstance(context.Background(), "", false)
			if err != nil {
				log.Fatal(fmt.Sprintf("error while terminating instance - %s", err.Error()))
			}
		}
	}()
}

func (o *handler) setCiTagTrue() {
	for i, tag := range o.tags {
		if *tag.Key == cloudInitTagKey {
			o.tags[i].Value = aws.String(fmt.Sprintf("%t", true))
		}
		return
	}
	o.tags = append(o.tags, types.Tag{
		Key:   aws.String(cloudInitTagKey),
		Value: aws.String(fmt.Sprintf("%t", true)),
	})
}

func (o *handler) makeFinalAmi(ctx context.Context) (*string, error) {
	dii := &ec2.DescribeInstancesInput{InstanceIds: []string{o.tempInstanceId}}
	dio, err := o.ec2Client.DescribeInstances(ctx, dii)
	if err != nil {
		return nil, fmt.Errorf("error describing stopped instance '%s' - %w", o.tempInstanceId, err)
	}
	if len(dio.Reservations) != 1 {
		return nil, fmt.Errorf("expected 1 reservation, describe instances found %d reservations", len(dio.Reservations))
	}
	r := dio.Reservations[0]
	if len(r.Instances) != 1 {
		return nil, fmt.Errorf("expected 1 instance, describe instances found %d instances", len(r.Instances))
	}
	i := r.Instances[0]
	if len(i.BlockDeviceMappings) != 1 {
		return nil, fmt.Errorf("expected 1 block device mapping, describe instances found %d block device mappings", len(i.BlockDeviceMappings))
	}
	m := i.BlockDeviceMappings[0]

	csi := &ec2.CreateSnapshotInput{
		VolumeId:    m.Ebs.VolumeId,
		Description: aws.String(fmt.Sprintf("Apstra server with cloud-init")),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeSnapshot,
			Tags:         o.tags,
		}},
	}
	cso, err := o.ec2Client.CreateSnapshot(ctx, csi)
	if err != nil {
		return nil, fmt.Errorf("error creating snapshot from instance '%s' - %w", o.tempInstanceId, err)
	}

	err = o.waitSnapshotComplete(ctx, cso.SnapshotId)
	if err != nil {
		return nil, fmt.Errorf("error checking for snapshot completion - %w", err)
	}

	rii := &ec2.RegisterImageInput{
		Name:         aws.String(nameFromTagsOrRandom(o.tags) + "-" + randString(6)),
		Architecture: types.ArchitectureValues(types.ArchitectureTypeX8664),
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sda1"),
			Ebs: &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				SnapshotId:          cso.SnapshotId,
				VolumeType:          types.VolumeTypeGp2,
			},
		}},
		Description:        aws.String("Apstra with cloud-init"),
		EnaSupport:         aws.Bool(true),
		ImdsSupport:        types.ImdsSupportValuesV20,
		RootDeviceName:     aws.String("/dev/sda1"),
		VirtualizationType: aws.String(string(types.VirtualizationTypeHvm)),
	}
	rio, err := o.ec2Client.RegisterImage(ctx, rii)
	if err != nil {
		return nil, fmt.Errorf("error importing snapshot - %w", err)
	}
	if rio == nil {
		return nil, errors.New("nil return from registerImage")
	}
	if rio.ImageId == nil {
		return nil, errors.New("nil ImageId return from registerImage")
	}

	_, err = o.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{*rio.ImageId},
		Tags:      o.tags,
	})
	if err != nil {
		return nil, fmt.Errorf("error while setting tags on new AMI '%s' - %w", *rio.ImageId, err)
	}

	log.Printf("new AMI with cloud-init is '%s'", *rio.ImageId)

	return rio.ImageId, nil
}

func (o *handler) terminateInstance(ctx context.Context, message string, messageIsErr bool) (*Response, error) {
	tii := &ec2.TerminateInstancesInput{InstanceIds: []string{o.tempInstanceId}}
	_, err := o.ec2Client.TerminateInstances(ctx, tii)
	if err != nil {
		return &Response{
			Message: message,
			Error:   fmt.Sprintf("termination of instance '%s' seems to have failed - please check it - %s", o.tempInstanceId, err.Error()),
		}, err
	}

	if messageIsErr {
		return &Response{Error: message}, errors.New(message)
	}

	return &Response{Message: message}, nil
}

func (o *handler) waitSnapshotComplete(ctx context.Context, snapshotId *string) error {
	log.Printf("waiting for snapshot '%s' completion", *snapshotId)
	dsi := &ec2.DescribeSnapshotsInput{
		SnapshotIds: []string{*snapshotId},
	}

	var dso *ec2.DescribeSnapshotsOutput
	var err error
	for {
		dso, err = o.ec2Client.DescribeSnapshots(ctx, dsi)
		if err != nil {
			return fmt.Errorf("error getting description of snapshot '%s' - %w", *snapshotId, err)
		}

		if len(dso.Snapshots) != 1 {
			return fmt.Errorf("expected 1 snapshot description, got %d descriptions", len(dso.Snapshots))
		}

		switch dso.Snapshots[0].State {
		case types.SnapshotStateCompleted:
			return nil
		case types.SnapshotStatePending:
			time.Sleep(apiIterationWait)
			continue
		default:
			return fmt.Errorf("snapshot '%s' in unexpected state: '%s'", *snapshotId, dso.Snapshots[0].State)
		}
	}
}

func (o *handler) cleanup(ctx context.Context) error {
	var err, iErr, sErr, aErr error
	if !o.keepInstance {
		_, iErr = o.ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{o.tempInstanceId}})
	}
	if iErr != nil {
		err = fmt.Errorf("error terminating temporary instance: %w", iErr)
	}

	if !o.keepAmi {
		_, aErr = o.ec2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
			ImageId: aws.String(o.tempAmiId),
		})
	}
	if aErr != nil {
		if err != nil {
			err = fmt.Errorf("%s - error deregistering temporary AMI - %w", err, aErr)
		} else {
			err = fmt.Errorf("error deregistering temporary AMI - %w", aErr)
		}
	}

	if !o.keepSnapshot {
		_, sErr = o.ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: aws.String(o.snapshotId),
			DryRun:     nil,
		})
	}
	if sErr != nil {
		if err != nil {
			err = fmt.Errorf("%s - error deleting temporary snapshot - %w", err, sErr)
		} else {
			err = fmt.Errorf("error deleting temporary snapshot - %w", sErr)
		}
	}

	return err
}

func nameFromTagsOrRandom(tags []types.Tag) string {
	for _, tag := range tags {
		if *tag.Key == "Name" {
			return *tag.Value
		}
	}
	return randString(6)
}

func randString(n int) string {
	rand.Seed(time.Now().UnixNano())
	data := make([]byte, n)
	rand.Read(data)
	return base64.URLEncoding.EncodeToString(data)
}
