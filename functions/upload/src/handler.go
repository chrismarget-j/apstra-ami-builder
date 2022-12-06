package upload

import (
	"context"
	"encoding/json"
	"fmt"
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
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	Tags []Tag  `json:"tags"`
}

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Response struct {
	Bucket string            `json:"bucket,omitempty"`
	Etags  map[string]string `json:"etags,omitempty"`
	Error  string            `json:"error,omitempty"`
}

func HandleRequest(request Request) (*Response, error) {
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

	faer, err := FetchAndExtract(context.Background(), FetchAndExtractRequest{
		Url:        request.Url,
		BucketName: bucketName,
		Files:      request.Files,
	})

	if err != nil {
		return nil, err
	}

	return &Response{
		Bucket: bucketName,
		Etags:  faer.Etags,
	}, nil
}
