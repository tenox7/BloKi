# BloKi TODO

## Major Features

- user comments
  - accounts: ???
  - spam: https://akismet.com/
- tags
- wiki mode
- consider switching to goldmark, see extensions like wiki mode links, etc
  https://github.com/yuin/goldmark?tab=readme-ov-file#extensions

## Core Engine

- render node hook for /media/
- wiki style links to post/media/etc
- "more" tag/continue reading refactor as ast node
- author and pub/mod date also render by gomarkdown
- render to static site
- render cache
- throttle qps
- statistics module, page views, latencies, etc
- reindex on inotify (incl rename/move)
- reindex on signal
- gcs, s3 support
- smart resize images for preview
  if only slightly larger than theme use img src size
  otherwise generate a preview version and cache on disk

## Runtimes

- cloud run, fargate, lambda, app runner, cloud functions
- windows service, app-v

## Admin

- better error messages than text
  perhaps route messages like for users.list()
- 2fa for admin login
  https://github.com/pquerna/otp
  https://github.com/xlzd/gotp
  https://www.twilio.com/docs/verify/quickstarts/totp
- stats
- fancy, 3rd party, javascript based markdown editor
- sort by different columns name/author/published/modified
- preview mode, render unpublished article with authentication
- admin publish/unpublish button with bytes.Replace()
- real dialogs instead of javascript popups
- alert on image size - over certain file size
- image resize: imaginary, bimg, disintegration
- users to have email addresses
  change password
  verification
  git commit
- git tab showing commit log, etc
- cleanup path names, short, full, unesacped, escaped, etc