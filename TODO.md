# BloKi TODO

## Major Features

- user comments
  - accounts: ???
  - spam: https://akismet.com/
- startup wizard
- wiki mode
- tags

## Core Engine

- git integration
- render node hook for /media/
- wiki style links to post/media/etc
- better error handling, use string builder
- render to static site
- render cache
- throttle qps
- statistics module, page views, latencies, etc
- reindex on inotify (incl rename/move)
- reindex on signal
- s3 support

## Runtimes

- docker container
- cloud run, fargate
- lambda, cloud functions
- fastcgi

## Admin

- 2fa for admin login
  https://github.com/pquerna/otp
  https://github.com/xlzd/gotp
  https://www.twilio.com/docs/verify/quickstarts/totp
- user manager
- better error messages than text
- stats
- fancy, 3rd party, javascript based markdown editor
- sort by different columns name/author/published/modified
- preview mode, render unpublished article with authentication
- admin publish/unpublish button with bytes.Replace()

## Tech Debt

- "more" tag/continue reading refactor as ast node
