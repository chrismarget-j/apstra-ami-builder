package toami

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"log"
)

const (
	vmdkFormat = "VMDK"
)

//type Request struct {
//}

type Response struct {
	Error string
}

type Request struct {
	Records []struct {
		S3 struct {
			Bucket struct {
				Arn  string `json:"arn"`
				Name string `json:"name"`
			} `json:"bucket"`
			Object struct {
				Key string `json:"key"`
			} `json:"object"`
		} `json:"s3"`
	} `json:"Records"`
}

func HandleRequest(ctx context.Context, request Request) (*Response, error) {
	dump, err := json.Marshal(request)
	if err != nil {
		err = fmt.Errorf("error unmarshaling request - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("request received: '%s'", string(dump))

	for i, record := range request.Records {
		log.Printf("parsing record %d", i)
		h, err := newVmdkHandler(ctx, record.S3.Bucket.Name, record.S3.Object.Key)
		if err != nil {
			return nil, fmt.Errorf("error creating vmdk handler - %w", err)
		}
		err = h.getTags(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting tags from S3 object - %w", err)
		}
		err = h.importSnapshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("error creating ebs snapshot - %w", err)
		}
	}
	return &Response{}, nil
}

type tag struct {
	key   string
	value string
}

type vmdkHandler struct {
	ec2Client *ec2.Client
	s3Client  *s3.Client
	bucket    string
	key       string
	tagSet    []s3Types.Tag
}

func newVmdkHandler(ctx context.Context, bucket string, key string) (*vmdkHandler, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading default AWS config - %w", err)
	}

	return &vmdkHandler{
		s3Client:  s3.NewFromConfig(awsCfg),
		ec2Client: ec2.NewFromConfig(awsCfg),
		bucket:    bucket,
		key:       key,
	}, nil
}

func (o *vmdkHandler) getTags(ctx context.Context) error {
	tagging, err := o.s3Client.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(o.key),
	})
	if err != nil {
		return fmt.Errorf("error getting tags on 's3://%s/%s' - %w", o.bucket, o.key, err)
	}
	o.tagSet = tagging.TagSet
	return nil
}

func (o *vmdkHandler) importSnapshot(ctx context.Context) error {
	log.Printf("importing snapshot")
	snapshotData, err := o.ec2Client.ImportSnapshot(ctx, &ec2.ImportSnapshotInput{
		ClientData:  nil,
		ClientToken: nil,
		Description: nil,
		DiskContainer: &ec2Types.SnapshotDiskContainer{
			Description: nil,
			Format:      aws.String(vmdkFormat),
			Url:         nil,
			UserBucket: &ec2Types.UserBucket{
				S3Bucket: aws.String(o.bucket),
				S3Key:    aws.String(o.key),
			},
		},
		DryRun:            nil,
		Encrypted:         nil,
		KmsKeyId:          nil,
		RoleName:          nil,
		TagSpecifications: nil,
	})
	if err != nil {
		return fmt.Errorf("error creating snapshot - %w", err)
	}
	log.Printf("snapshot import task id: " + *snapshotData.ImportTaskId)
	return nil
}
