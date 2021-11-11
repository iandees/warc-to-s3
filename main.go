package main

import (
	"bufio"
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/slyrz/warc"
)

func main() {
	inputFilename := flag.String("input", "", "The input WARC to use")
	outputBucket := flag.String("bucket", "", "The S3 bucket to upload to")
	concurrency := flag.Int("concurrency", 16, "The number of concurrent uploads to S3")
	addWarningBanner := flag.Bool("add-banner", false, "Add a warning banner to the top of every HTML page")
	flag.Parse()

	if *inputFilename == "" {
		log.Fatalf("--input is required")
	}

	if *outputBucket == "" {
		log.Fatalf("--bucket is required")
	}

	openedFile, err := os.Open(*inputFilename)
	if err != nil {
		log.Fatalf("Error opening file: %+v", err)
	}
	defer openedFile.Close()

	reader, err := warc.NewReader(openedFile)
	if err != nil {
		log.Fatalf("Couldn't open WARC: %+v", err)
	}
	defer reader.Close()

	sess := session.Must(session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable}))

	postToS3 := make(chan *http.Response, 100)
	uploaderWG := &sync.WaitGroup{}
	for i := 0; i < *concurrency; i++ {
		uploaderWG.Add(1)
		go func() {
			defer uploaderWG.Done()
			uploader := s3manager.NewUploader(sess)
			for resp := range postToS3 {
				key := resp.Request.URL.Path

				if strings.HasSuffix(key, "/") {
					// Put the output of a bare URL at index.html so S3 websites will serve it
					key += "index.html"
				}

				// Chop off the leading slash
				key = key[1:]

				all, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("Error reading body as HTML: %+v", err)
				}

				// Optionally modify the content to add a deprecation header if it's HTML
				contentType := resp.Header.Get("content-type")
				if *addWarningBanner && contentType == "text/html; charset=utf-8" {
					replaced := strings.Replace(string(all), "<body>", "<body><div style=\"margin:0;padding:5px;width:100%;background:#a00;color:#fff\">⚠️&nbsp;<strong>This is a static archive and is no longer maintained.</div>", 1)
					all = []byte(replaced)
				}

				uploadInput := &s3manager.UploadInput{
					Bucket:      aws.String(*outputBucket),
					Key:         aws.String(key),
					ContentType: aws.String(contentType),
					Body:        bytes.NewReader(all),
				}
				_, err = uploader.Upload(uploadInput)
				if err != nil {
					log.Printf("Error uploading s3://%s/%s to S3: %+v", *outputBucket, key, err)
					resp.Body.Close()
					continue
				}

				resp.Body.Close()

				log.Printf("Uploaded to s3://%s/%s", *outputBucket, key)
			}
		}()
	}

	var request *http.Request
	for {
		record, err := reader.ReadRecord()
		if err != nil {
			log.Printf("Can't read WARC record: %+v", err)
			break
		}

		if record.Header.Get("content-type") == "application/warc-fields" {
			// Skip the WARC header
			continue
		}

		all, err := ioutil.ReadAll(record.Content)
		if err != nil {
			break
		}

		b := bufio.NewReader(bytes.NewReader(all))

		if record.Header.Get("content-type") == "application/http;msgtype=request" {
			request, err = http.ReadRequest(b)
			if err != nil {
				log.Printf("Can't parse record HTTP request: %+v", err)
				break
			}
		} else if record.Header.Get("content-type") == "application/http;msgtype=response" {
			response, err := http.ReadResponse(b, request)
			if err != nil {
				log.Printf("Can't parse record HTTP response: %+v", err)
				break
			}

			postToS3 <- response
			request = nil
		}
	}
	close(postToS3)

	uploaderWG.Wait()
}
