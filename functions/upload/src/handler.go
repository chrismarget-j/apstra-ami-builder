package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"log"
	"os"
)

const (
	bucketNameEnv = "BUCKET_NAME"
)

type Request struct {
	Url   string      `json:"url"`
	Files []S3ObjInfo `json:"files"`
}

type S3ObjInfo struct {
	Src  *string `json:"src"`
	Dst  *string `json:"dst"`
	Tags []Tag   `json:"tags"`
}

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Response struct {
	Bucket  string   `json:"bucket,omitempty"`
	TaskIds []string `json:"task_ids,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func HandleRequest(ctx context.Context, request Request) (*Response, error) {
	dump, err := json.Marshal(request)
	if err != nil {
		err = fmt.Errorf("error unmarshaling request - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("request received: '%s'", string(dump))

	bucketName, found := os.LookupEnv(bucketNameEnv)
	if !found {
		err := fmt.Errorf("environment variable '%s' not set", bucketNameEnv)
		return &Response{Error: err.Error()}, err
	}
	if bucketName == "" {
		err := fmt.Errorf("environment variable '%s' empty", bucketNameEnv)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("files will be extracked to bucket '%s'", bucketName)

	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		err := fmt.Errorf("error loading aws config - %w", err)
		return &Response{Error: err.Error()}, err
	}

	faer, err := fetchAndExtract(ctx, fetchAndExtractRequest{
		awsConfig:  awsConfig,
		url:        request.Url,
		bucketName: bucketName,
		files:      request.Files,
	})
	if err != nil {
		return nil, err
	}

	taskIds := make([]string, len(faer.items))
	for i, item := range faer.items {
		siri := snapshotImportRequestInput{
			awsConfig: awsConfig,
			s3Bucket:  item.s3bucket,
			s3Key:     item.s3key,
		}
		siro, err := snapshotImport(ctx, siri)
		if err != nil {
			err := fmt.Errorf("error while importing snapshot - %w", err)
			return &Response{Error: err.Error()}, err
		}
		taskIds[i] = siro.taskId
	}

	return &Response{
		Bucket:  bucketName,
		TaskIds: taskIds,
	}, nil
}
