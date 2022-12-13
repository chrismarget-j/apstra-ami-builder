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
	//"github.com/chrismarget-j/apstra-ami-builder/functions/cloudinit"
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
	msgSuccess     = "clean exit"

	envVarInstallCiLambdaName        = "INSTALL_CI_LAMBDA_NAME"
	envVarInstallCiLambdaSG          = "INSTALL_CI_LAMBDA_SECURITY_GROUP"
	envVarInstanceType               = "INSTANCE_TYPE"
	envVarRetainIntermediateSnapshot = "KEEP_INTERMEDIATE_SNAPSHOT"
	envVarRetainIntermediateAmi      = "KEEP_INTERMEDIATE_AMI"

	ec2InstanceIterationWait = 500 * time.Millisecond
	ec2InstanceIterationsMax = 30
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
}

func HandleRequest(ctx context.Context, request *Request) (*Response, error) {
	h, err := newHandler(ctx, request)
	if err != nil {
		err = fmt.Errorf("error creating new handler - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.testNextStage(ctx)
	if err != nil {
		err = fmt.Errorf("error testing next lambda - %w", err)
		return &Response{Error: err.Error()}, err
	}

	err = h.filterEvents()
	if err != nil {
		var hErr handlerErr
		if errors.As(err, &hErr) && hErr.Type() == errFilterFail {
			log.Printf("filter says nope")
			return &Response{Message: msgIgnoreEvent}, nil
		}
		log.Printf("some other error: '%s'", err.Error())
		err = fmt.Errorf("error filtering incoming event - %w", err)
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
	dump, _ := json.Marshal(h.task.Tags)
	log.Printf("snapshot '%s' tagged: '%s'", h.snapshot, string(dump))

	err = h.makeTempAmi(ctx)
	if err != nil {
		err = fmt.Errorf("error creating temporary AMI - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("temporary AMI '%s' created", h.tempAmiId)

	id, err := h.bootTempAmi(ctx)
	if err != nil {
		err = fmt.Errorf("error starting temporary instance from AMI - %w", err)
		return &Response{Error: err.Error()}, err
	}

	ip, err := h.waitPublicIp(ctx, id)
	if err != nil {
		err = fmt.Errorf("error waiting for ec2 public IP - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("public ip: '%s'", *ip)

	err = h.waitUntilStopped(ctx, id)
	if err != nil {
		_, _ = h.ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{*id}})
		err = fmt.Errorf("error waiting for instance to stop - %w", err)
		return &Response{
			Message: fmt.Sprintf("terminating instance '%s'", *id),
			Error:   err.Error(),
		}, err
	}

	return &Response{Message: msgSuccess}, nil
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

	dump, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling request - %w", err)
	}

	log.Printf("request received: '%s'", string(dump))

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading default AWS config - %w", err)
	}

	snapshot, err := arn.Parse(request.Detail.SnapshotId)
	if err != nil {
		return nil, fmt.Errorf("error parsing snapshot arn ('%s') found in request - %w", request.Detail.SnapshotId, err)
	}

	return &handler{
		ec2Client:            ec2.NewFromConfig(awsCfg),
		lambdaClient:         lambda.NewFromConfig(awsCfg),
		request:              request,
		snapshot:             snapshot,
		installCiSecurityGrp: securityGroup,
		installCiLambdaName:  installCiLambda,
		instanceType:         instanceType,
		keepAmi:              os.Getenv(envVarRetainIntermediateAmi) == "true",
		keepSnapshot:         os.Getenv(envVarRetainIntermediateSnapshot) == "true",
	}, nil
}

type handler struct {
	request              *Request
	ec2Client            *ec2.Client
	lambdaClient         *lambda.Client
	task                 *types.ImportSnapshotTask
	snapshot             arn.ARN
	tempAmiId            string
	name                 string
	instanceType         string
	installCiLambdaName  string
	installCiSecurityGrp string
	keepSnapshot         bool
	keepAmi              bool
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
	params := &ec2.DescribeImportSnapshotTasksInput{
		DryRun:        nil,
		Filters:       nil,
		ImportTaskIds: nil,
		MaxResults:    nil,
		NextToken:     nil,
	}
	paginator := ec2.NewDescribeImportSnapshotTasksPaginator(o.ec2Client, params)

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
			if *task.SnapshotTaskDetail.SnapshotId == path.Base(o.snapshot.Resource) {
				o.task = &task
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
		Resources: []string{path.Base(o.snapshot.String())},
		Tags:      o.task.Tags,
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
		Name:         aws.String(nameFromTagsOrRandom(o.task.Tags) + "-" + randString(6)),
		Architecture: "x86_64",
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sda1"),
			Ebs: &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				SnapshotId:          aws.String(path.Base(o.snapshot.Resource)),
				VolumeType:          "gp2",
			},
		}},
		Description:        aws.String("scratchpad AMI for cloud-init installation"),
		EnaSupport:         aws.Bool(true),
		ImdsSupport:        "v2.0",
		RootDeviceName:     aws.String("/dev/sda1"),
		SriovNetSupport:    nil,
		VirtualizationType: aws.String("hvm"),
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
		Tags:      o.task.Tags,
	})

	return nil
}

func (o *handler) bootTempAmi(ctx context.Context) (*string, error) {
	rii := &ec2.RunInstancesInput{
		MaxCount:                          aws.Int32(1),
		MinCount:                          aws.Int32(1),
		ImageId:                           aws.String(o.tempAmiId),
		InstanceInitiatedShutdownBehavior: types.ShutdownBehaviorStop,
		InstanceType:                      types.InstanceType(o.instanceType),
		SecurityGroupIds:                  []string{o.installCiSecurityGrp},
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeInstance,
			Tags:         o.task.Tags,
		}},
	}
	rio, err := o.ec2Client.RunInstances(ctx, rii)
	if err != nil {
		return nil, fmt.Errorf("error running temporary EC2 instance for cloud-init installation - %w", err)
	}
	if len(rio.Instances) != 1 {
		dump, _ := json.Marshal(rio)
		return nil, fmt.Errorf("expected to start 1 instance, got %d instances - full output - %s", len(rio.Instances), string(dump))
	}
	log.Printf("launched instance '%s', waiting for public IP to appear...", *rio.Instances[0].InstanceId)

	return rio.Instances[0].InstanceId, nil
}

func (o *handler) getPublicIp(ctx context.Context, id *string) (*string, error) {
	instance, err := o.getInstance(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error getting instance while determining public IP - %w", err)
	}
	return instance.PublicIpAddress, nil
}

func (o *handler) waitPublicIp(ctx context.Context, id *string) (*string, error) {
	var i int
	for i < ec2InstanceIterationsMax {
		i++
		ip, err := o.getPublicIp(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("error while waiting for public IP - %w", err)
		}
		if ip != nil {
			return ip, nil
		}
		time.Sleep(ec2InstanceIterationWait)
	}
	return nil, fmt.Errorf("timeout waiting for public IP to appear on '%s'", *id)
}

func (o *handler) waitUntilStopped(ctx context.Context, id *string) error {
	log.Printf("waiting for instance '%s' to stop...", *id)
	var i int
	for i < ec2InstanceIterationsMax {
		instance, err := o.getInstance(ctx, id)
		if err != nil {
			return fmt.Errorf("error getting instance state while waiting for it to stop")
		}
		if instance.State.Name == types.InstanceStateNameStopped {
			return nil
		}
		time.Sleep(ec2InstanceIterationWait)
	}
	return fmt.Errorf("instance '%s' didn't stop in the alotted time", *id)
}

func (o *handler) getInstance(ctx context.Context, id *string) (*types.Instance, error) {
	params := &ec2.DescribeInstancesInput{
		InstanceIds: []string{*id},
	}
	paginator := ec2.NewDescribeInstancesPaginator(o.ec2Client, params)
	for paginator.HasMorePages() {
		var i int
		var err error
		var dio *ec2.DescribeInstancesOutput
		for i < ec2InstanceIterationsMax {
			i++
			dio, err = paginator.NextPage(ctx)
			if err == nil {
				break
			}
			time.Sleep(ec2InstanceIterationWait)
		}
		if err != nil {
			return nil, fmt.Errorf("error getting instance descriptions from paginator - %w", err)
		}
		return &dio.Reservations[0].Instances[0], nil
	}
	return nil, fmt.Errorf("lone instance not found in first page of results")
}

func (o *handler) testNextStage(ctx context.Context) error {
	//json.Marshal(&cloudinit.Request{Oper})

	ii := &lambda.InvokeInput{
		FunctionName:   aws.String(o.installCiLambdaName),
		ClientContext:  nil,
		InvocationType: "",
		LogType:        "",
		Payload:        []byte("{\"operation\": \"ping\"}"),
		Qualifier:      nil,
	}
	io, err := o.lambdaClient.Invoke(ctx, ii)
	if err != nil {
		return fmt.Errorf("error invoking '%s' lambda - %w", o.installCiLambdaName, err)
	}
	log.Printf(string(io.Payload))
	return nil
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
