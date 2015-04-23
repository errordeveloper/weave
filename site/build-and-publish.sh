#!/bin/sh

cd $(dirname $0)/

## Use GCS until we set up Route53/CloudFront/S3

branch=`git rev-parse --abbrev-ref=strict HEAD`
commit=`git rev-parse HEAD`

bundle install --path=.bundle
bundle exec jekyll build --verbose
gsutil cp -z html,css -a public-read -R _site gs://docs.weave.works/weave/${branch}/${commit}
