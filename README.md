# BloKi - A minimalistic Blog and Wiki Engine

If you want a small, simple, easy to use, high performance, micro blog/wiki platform and you're tired of, or afraid to even touch WordPress, MediaWiki, LAMP, this might be for you.

## Main features

- No PHP, Apache, Nginx, JavaScript and SQL.

- The web based Admin UI resembles WordPress and MediaWiki, but without all the bloat.

- Articles and media are stored as regular files. Apart from web admin - editable in any editor of your choice! Versioned by Git.

- The server software is a single file, statically linked, self contained binary. Runs on most modern OSes without any external dependencies, scripting runtimes, libraries, etc. Easily host your blog on a Raspberry PI, small VM, Docker container, Lambda or Cloud Run function.

- BloKi supports modern, low end, legacy, vintage and even text mode browsers. User editable themes.


## Current status

- Basic blog engine and admin interface works.
- You can see working example [here](https://blog.tenox.net/)

Development progress can be tracked on [dogfood blog](https://blog.tenox.net/)

## Running BloKi

Sample systemd configuration files are provided. Similar to any other web server, BloKi will require
either a privileged account or set of capabilities to bind to port 80 and 443. When using the secrets
file, it is recommended to start BloKi as root with `-chroot` and `-setuid` flags. This way BloKi can
open the secrets store before entering chroot. However you can also chroot and setuid from systemd.

## Legal
Copyright (c) 2024 by Antoni Sawicki
Licensed under Apache-2.0 license