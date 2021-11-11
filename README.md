# warc-to-s3

This is a small Go application that consumes a [WARC](https://en.wikipedia.org/wiki/Web_ARChive) file (
using [`slyzrc/warc`](https://github.com/slyrz/warc)) and puts it on S3 suitable for serving
with [S3 Website Hosting](https://docs.aws.amazon.com/AmazonS3/latest/userguide/WebsiteHosting.html).

## Generating a WARC

There are all sorts of ways to generate a WARC, but one way is to use `wget` like so:

```
wget --recursive \
     --warc-file=example.com \
     --warc-compression \
     --delete-after \
     --no-directories \
     --reject-regex "runs/.*/sample.html" \
     --tries=3 \
     --retry-on-http-error=500,502 \
     --domains=example.com \
      https://example.com/
```

This will generate a file called `example.com.warc.gz`, which you can then pass into this program.

## Warning banner

The code will introduce a red warning banner saying that the content is archived if you set the `--add-banner` flag at
runtime. Check out the code for how that works.
