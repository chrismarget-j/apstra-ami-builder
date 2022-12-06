package toami

import (
	"encoding/json"
	"fmt"
	"log"
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

func HandleRequest(request Request) (*Response, error) {
	dump, err := json.Marshal(request)
	if err != nil {
		err = fmt.Errorf("error unmarshaling request - %w", err)
		return &Response{Error: err.Error()}, err
	}
	log.Printf("request received: '%s'", string(dump))

	return &Response{}, nil
}
