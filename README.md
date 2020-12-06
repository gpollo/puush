# puush

This repository contains files for a self-hosted _API-only_ alternative to [puush.me](https://puush.me/).

## Usage

To use the service, a session must first be created. It can be done with the following command:

```bash
$ curl -X GET http://example.com/api/session
3a42f234-2466-4fbf-8a64-61393eb380ac
```

A file can then be uploaded using the following command (example: `main/main.go`):

```bash
$ curl -X POST \
       --cookie "SESSION_KEY=3a42f234-2466-4fbf-8a64-61393eb380ac" \
       --form "file=@main/main.go" \
       http://example.com/api/upload
example.com/lfz.go
```

The file is now accessible at the returned URL `example.com/lfz.go`. It can then be shared, open
on the web or downloaded with the following command:

```bash
$ curl -OJ example.com/lfz.go
...
curl: Saved to filename 'main.go'
```

## Alias

If you want handy commands for puush-ing files and automatically put the returned URL in the
clipboard, you can add these functions into your `~/.bashrc` or `~/.zshrc` (requires `xsel` and
`jq`):

```bash
function puush() {
    local filename="$1"
    local fileurl

    fileurl=$(
        curl -X POST --fail --silent --show-error \
            --cookie "SESSION_KEY=3a42f234-2466-4fbf-8a64-61393eb380ac" \
            --form "file=@$filename" \
            http://example.com/api/upload
    )

    if [[ -n "$DISPLAY" ]]; then
        # works only for X server
        xsel --clipboard <<< "$fileurl"
    else
        # fallback if you're in SSH or Wayland
        echo "$fileurl"
    fi
}

function puushls() {
    curl -X GET --fail --silent --show-error \
        --cookie "SESSION_KEY=$PUUSH_API_KEY" \
        "https://files.gpol.sh/api/list" | jq .
}

function puushrm() {
    local fileid

    for fileid in "$@"; do
        curl -X DELETE --fail --silent --show-error \
            --cookie "SESSION_KEY=$PUUSH_API_KEY" \
            "https://files.gpol.sh/$fileid"
    done
}
```

It gives the following commands:

* `puush PATH/TO/FILE` uploads and copies to clipboard if possible
* `puushls` lists all your files in JSON format
* `puushrm [ID]...` delete files using their IDs

## Deployment

The Docker image [gpol/puush](https://hub.docker.com/r/gpol/puush) can be used for deployement
along side a PostgreSQL database.

The image provides the following environment variables:

| Variable                    | Required | Description                                       |
| --------------------------- | -------- |-------------------------------------------------- |
| `PUUSH_POSTGRESQL_USER`     | Yes      | PostgreSQL user.                                  |
| `PUUSH_POSTGRESQL_PASS`     | Yes      | PostgreSQL user password.                         |
| `PUUSH_POSTGRESQL_HOST`     | Yes      | PostgreSQL host.                                  |
| `PUUSH_POSTGRESQL_DATABASE` | Yes      | PostgreSQL database name.                         |
| `PUUSH_ROOT_DIRECTORY`      | No       | Where to store the files. _Default: `/srv/puush`_ |

The server can be easily deployed using the following [docker-compose.yaml](docker-compose.yaml).

## Reverse proxy using NGINX

You might want to use a reverse proxy such as NGINX to [enable SSL](shorturl.at/mFOR8) or such.

### Required headers

Let's assume that we want our service to be hosted on `files.example.com` using SSL. The following
`location` block can be used:

```nginx
location / {
    proxy_pass http://puush:8080/;
    proxy_set_header Host files.example.com;
    proxy_set_header Forwarded https;
    proxy_pass_request_headers on;
}
```

The `Host` header must be used in order to return a URL of `files.example.com` and not `puush:8080`.

The `Forwarded` header must be used if the protocol is `https://` or else it will return a URL
starting with `http://`.

### Restricting session creation

You can restrict session creation by adding the following `location` block:

```nginx
location /api/session {
    deny all;
}
```

You can then use the following Docker command to generate a session:

```bash
$ docker exec -it puush curl -X GET http://127.0.0.1:8080/api/session
f83f8721-9ce5-4de8-b8eb-19e8933ecd95
```

### Limiting/Increasing file size

You can add the following settings in your configuration file to limit the size of the files:

```nginx
client_max_body_size 50M;
proxy_request_buffering off;
```

## TODOs

* optional file expiration
* delete entire session
