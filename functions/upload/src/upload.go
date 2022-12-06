package upload

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
	Files      []S3ObjInfo
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

	dstMap := make(map[string]struct{}, len(o.Files))
	srcMap := make(map[string]struct{}, len(o.Files))
	for _, s3ObjInfo := range o.Files {
		if s3ObjInfo.Src == "" {
			return errors.New("validation error: blank archive filename detected")
		}
		if s3ObjInfo.Dst == "" {
			return errors.New("validation error: blank s3 target key detected")
		}
		if _, found := srcMap[s3ObjInfo.Src]; found {
			return fmt.Errorf("validation error: archive filename '%s' specified more than once", s3ObjInfo.Src)
		}
		if _, found := dstMap[s3ObjInfo.Dst]; found {
			return fmt.Errorf("validation error: S3 target key '%s' specified more than once", s3ObjInfo.Dst)
		}
		dstMap[s3ObjInfo.Dst] = struct{}{}
		srcMap[s3ObjInfo.Src] = struct{}{}
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
		url:    req.Url,
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
	url    string
	src    io.ReadCloser
	bucket string
	files  []S3ObjInfo
	etags  map[string]string
}

func extractSelectedToS3(ctx context.Context, req *extractSelectedToS3Request) error {
	defer req.src.Close()

	mapByArchiveName := mapS3ObjInfoBySrcFile(req.files)

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
		if s3ObjInfo, found := mapByArchiveName[header.Name]; found {
			log.Printf("match found: '%s'\n", header.Name)
			log.Printf("extracting %d bytes from archive\n", header.Size)
			foundInTar[header.Name] = struct{}{}
			apiResponse, err := extractFileToS3(ctx, extractFileToS3Request{
				reader:   tarReader,
				len:      header.Size,
				bucket:   req.bucket,
				s3Client: s3Client,
				src:      s3ObjInfo.Src,
				dst:      s3ObjInfo.Dst,
				tags:     s3ObjInfo.Tags,
			})
			if err != nil {
				return fmt.Errorf("error extracing file to s3 - %w", err)
			}
			req.etags[s3ObjInfo.Dst] = apiResponse.etag
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
	for _, s3ObjInfo := range req.files {
		if _, found := foundInTar[s3ObjInfo.Src]; !found {
			missing[i] = s3ObjInfo.Src
			i++
		}
	}
	return fmt.Errorf("requested files not found in archive: '%s'", strings.Join(missing, "', '"))
}

func mapS3ObjInfoBySrcFile(in []S3ObjInfo) map[string]*S3ObjInfo {
	result := make(map[string]*S3ObjInfo, len(in))
	for i, s3ObjInfo := range in {
		result[s3ObjInfo.Src] = &in[i]
	}
	return result
}

type extractFileToS3Request struct {
	reader   io.Reader
	src      string
	dst      string
	tags     []Tag
	len      int64
	bucket   string
	s3Client *s3.Client
}

type extractFileToS3Response struct {
	etag string
}

func extractFileToS3(ctx context.Context, req extractFileToS3Request) (*extractFileToS3Response, error) {
	tags := make([]string, len(req.tags))
	for i, tag := range req.tags {
		tags[i] = url.QueryEscape(tag.Key) + "=" + url.QueryEscape(tag.Value)
	}

	params := &s3.PutObjectInput{
		Bucket:        aws.String(req.bucket),
		Key:           aws.String(req.dst),
		Body:          req.reader,
		ContentLength: req.len,
		Tagging:       aws.String(strings.Join(tags, "&")),
	}
	apiResponse, err := req.s3Client.PutObject(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error putting s3 object - %w", err)
	}

	return &extractFileToS3Response{
		etag: *apiResponse.ETag,
	}, nil
}
