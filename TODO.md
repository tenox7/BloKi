# BloKi TODO

## Major Features

- better modern template
  - constant make width - 1000px in WP?
  - wp default theme
    https://wordpress.org/themes/default/
  - twenty eleven theme
    https://wordpress.org/themes/twentyeleven/
- user comments
  - accounts: ???
  - spam: https://akismet.com/
- startup wizard
- wiki mode
- tags

## Core Engine

- render node hook for /media/
- wiki style links to post/media/etc
- git integration
- better error handling, use string builder
- s3 support
- render to static site
- render cache
- throttle qps
- statistics module, page views, latencies, etc
- reindex on inotify (incl rename/move)
- reindex on signal

## Runtimes

- docker container
- cloud run, fargate
- lambda, cloud functions
- fastcgi

## Admin

- admin publish/unpublish button with bytes.Replace()
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