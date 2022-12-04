package apstraami

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"log"

	"io"
	"net/http"
	"net/url"
	"strings"
)

type FetchAndExtractRequest struct {
	Url        string
	Files      map[string]string
	HttpClient *http.Client
	BucketName string
}

type FetchAndExtractResponse struct {
	Etags map[string]string
}

func (o FetchAndExtractRequest) validate() error {
	if o.Url == "" {
		return errors.New("validation error: url cannot be empty")
	}
	if _, err := url.Parse(o.Url); err != nil {
		return fmt.Errorf("validation error parsing url - %w", err)
	}
	if len(o.Files) == 0 {
		return errors.New("validation error: no files requested for extraction")
	}

	dstMap := make(map[string]struct{})
	for k, v := range o.Files {
		if k == "" {
			return errors.New("validation error: blank archive filename detected")
		}
		if v == "" {
			return errors.New("validation error: blank s3 target key detected")
		}
		if _, found := dstMap[v]; found {
			return fmt.Errorf("validation error: target '%s' specified more than once", v)
		}
		dstMap[v] = struct{}{}
	}
	return nil
}

func FetchAndExtract(ctx context.Context, req FetchAndExtractRequest) (*FetchAndExtractResponse, error) {
	err := req.validate()
	if err != nil {
		return nil, err
	}

	body, err := doHttp(ctx, doHttpRequest{
		url:    req.Url,
		client: req.HttpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("error during HTTP transaction - %w", err)
	}

	estr := extractSelectedToS3Request{
		src:    body,
		bucket: req.BucketName,
		files:  req.Files,
		etags:  make(map[string]string),
	}
	err = extractSelectedToS3(ctx, &estr)
	if err != nil {
		return nil, err
	}

	return &FetchAndExtractResponse{Etags: estr.etags}, nil
}

type doHttpRequest struct {
	url    string
	client *http.Client
}

func (o doHttpRequest) httpClient() *http.Client {
	if o.client == nil {
		return &http.Client{}
	}
	return o.client
}

func doHttp(ctx context.Context, req doHttpRequest) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating http request - %w", err)
	}

	log.Println("start http transaction")
	httpResponse, err := req.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error performing http request - %w", err)
	}

	if httpResponse.StatusCode/100 != 2 {
		httpResponse.Body.Close()
		return nil, fmt.Errorf("http response code %d from server", httpResponse.StatusCode)
	}

	log.Printf("http status code: %d", httpResponse.StatusCode)
	log.Printf("http content length: %d", httpResponse.ContentLength)
	log.Printf("http content type: '%s'", httpResponse.Header.Get("content-type"))

	return httpResponse.Body, nil
}

type extractSelectedToS3Request struct {
	src    io.ReadCloser
	bucket string
	files  map[string]string // map[archivePath]bucketPath
	etags  map[string]string
}

func extractSelectedToS3(ctx context.Context, req *extractSelectedToS3Request) error {
	defer req.src.Close()

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("error loading default AWS config - %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	// map to keep track of files we've found and detect duplicates
	foundInTar := make(map[string]struct{})

	tarReader := tar.NewReader(req.src)
	// loop through archive entries
	log.Println("looping over files in archive")
	for {
		// quit if we're cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("bailing out while parsing archive - %w", ctx.Err())
		default:
		}

		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return fmt.Errorf("error parsing tar data - %w", err)
			}
		}

		// duplicate entry?
		if _, found := foundInTar[header.Name]; found {
			return fmt.Errorf("extract error: multiple instances of file '%s'", header.Name)
		}

		// interesting file?
		if bucketKey, found := req.files[header.Name]; found {
			log.Printf("good one: '%s'\n", header.Name)
			foundInTar[header.Name] = struct{}{}
			apiResponse, err := extractFileToS3(ctx, extractFileToS3Request{
				src:      tarReader,
				len:      header.Size,
				bucket:   req.bucket,
				key:      bucketKey,
				s3Client: s3Client,
			})
			if err != nil {
				return fmt.Errorf("error extracing file to s3 - %w", err)
			}
			req.etags[bucketKey] = apiResponse.etag
		} else {
			log.Printf("skipping '%s'\n", header.Name)
		}
	}

	if len(req.files) == len(foundInTar) {
		// files requested and files found are the same size -- good result
		return nil
	}

	// some files were not found -- but which ones?
	missing := make([]string, len(req.files)-len(foundInTar))
	var i int
	for k := range req.files {
		if _, found := foundInTar[k]; !found {
			missing[i] = k
			i++
		}
	}
	return fmt.Errorf("requested files not found in archive: '%s'", strings.Join(missing, "', '"))
}

type extractFileToS3Request struct {
	src      io.Reader
	len      int64
	bucket   string
	key      string
	s3Client *s3.Client
}

type extractFileToS3Response struct {
	etag string
}

func extractFileToS3(ctx context.Context, req extractFileToS3Request) (*extractFileToS3Response, error) {
	params := &s3.PutObjectInput{
		Bucket:        aws.String(req.bucket),
		Key:           aws.String(req.key),
		Body:          req.src,
		ContentLength: req.len,
	}
	apiResponse, err := req.s3Client.PutObject(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error putting s3 object - %w", err)
	}

	return &extractFileToS3Response{
		etag: *apiResponse.ETag,
	}, nil
}
