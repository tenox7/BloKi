# BloKi TODO

## Major Features

- search posts
- user comments
  https://akismet.com/
- startup wizard
- wiki mode
- user admin
- stats

## Core Engine

- index update for a single file + re-run sequence
- make admin save/rename/delete also use that
- git integration
- better error handling, use string builder
- render node hook for /media/
- wiki style links to post/media/etc
- continue reading block element inside a post like in WP
- s3 support
- render to static site
- throttle qps
- statistics module, page views, latencies, etc
- constant make width - 1000px in WP?
- reindex on inotify (incl rename/move)
- rendered html article cache
- reindex on signal

## Runtimes

- docker container
- cloud run, fargate
- lambda, cloud functions
- fastcgi

## Admin

- sort by different columns name/author/published/modified
- search posts
- 2fa for admin login
  https:www.twilio.com/docs/verify/quickstarts/totp
- user manager
- better error messages than text
