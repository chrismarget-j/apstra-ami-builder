package upload

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"log"
)

type snapshotImportRequestInput struct {
	awsConfig aws.Config
	s3Bucket  string
	s3Key     string
}

type snapshotImportRequestOutput struct {
	taskId string
}

func snapshotImport(ctx context.Context, req snapshotImportRequestInput) (*snapshotImportRequestOutput, error) {
	s3url := fmt.Sprintf("s3://%s/%s", req.s3Bucket, req.s3Key)
	log.Printf("attempting snapshot import of '%s.", s3url)

	s3client := s3.NewFromConfig(req.awsConfig)
	goti := &s3.GetObjectTaggingInput{
		Bucket: aws.String(req.s3Bucket),
		Key:    aws.String(req.s3Key),
	}
	tagging, err := s3client.GetObjectTagging(ctx, goti)
	if err != nil {
		return nil, fmt.Errorf("error getting object tags - %w", err)
	}

	tags := make([]ec2Types.Tag, len(tagging.TagSet))
	for i, tag := range tagging.TagSet {
		tags[i] = ec2Types.Tag{
			Key:   tag.Key,
			Value: tag.Value,
		}
	}

	ec2Client := ec2.NewFromConfig(req.awsConfig)

	isi := &ec2.ImportSnapshotInput{
		Description: aws.String(fmt.Sprintf("import '%s'", s3url)),
		DiskContainer: &ec2Types.SnapshotDiskContainer{
			Description: aws.String(fmt.Sprintf("import '%s'", s3url)),
			Format:      aws.String(string(ec2Types.DiskImageFormatVmdk)),
			UserBucket: &ec2Types.UserBucket{
				S3Bucket: aws.String(req.s3Bucket),
				S3Key:    aws.String(req.s3Key),
			},
		},
		RoleName: roleName(),
		TagSpecifications: []ec2Types.TagSpecification{{
			ResourceType: ec2Types.ResourceTypeImportSnapshotTask,
			Tags:         tags,
		}},
	}

	iso, err := ec2Client.ImportSnapshot(ctx, isi)
	if err != nil {
		return nil, fmt.Errorf("error creating snapshot - %w", err)
	}
	if iso == nil {
		return nil, errors.New("nil ImportSnapshotOutput from ec2.ImportSnapshot")
	}
	if iso.ImportTaskId == nil {
		return nil, errors.New("nil ImportSnapshotOutput.ImportTaskId from ec2.ImportSnapshot")
	}

	return &snapshotImportRequestOutput{
		taskId: *iso.ImportTaskId,
	}, nil
}
