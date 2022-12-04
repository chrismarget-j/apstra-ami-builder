package apstraami

import (
	"context"
	"fmt"
	"log"
	"os"
)

const (
	bucketNameEnv = "BUCKET_NAME"
)

type Request struct {
	Url     string            `json:"url"`
	FileMap map[string]string `json:"file_map"`
	//Tags       map[string]string `json:"tag_map"`
}

type Response struct {
	Bucket string            `json:"bucket,omitempty"`
	Etags  map[string]string `json:"etags,omitempty"`
	Error  string            `json:"error,omitempty"`
}

func HandleRequest(request Request) (*Response, error) {
	log.Printf("request received: '%s'", request)
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

	faer, err := FetchAndExtract(context.TODO(), FetchAndExtractRequest{
		Url:        request.Url,
		BucketName: bucketName,
		Files:      request.FileMap,
	})

	if err != nil {
		return nil, err
	}

	return &Response{
		Bucket: bucketName,
		Etags:  faer.Etags,
	}, nil
}
