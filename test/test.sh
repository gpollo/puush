#!/usr/bin/bash

HOST="http://127.0.0.1:8080"
SCRIPT="$PWD/$(basename "$0")"

function log_test() {
    local text="$1"

    printf "test: $(tput setaf 3)${text}$(tput sgr0)\n"
}

function log_error() {
    local text="$1"

    printf "fail: $(tput setaf 1)${text}$(tput sgr0)\n"
}

function log_success() {
    local text="$1"

    printf "ok: $(tput setaf 2)${text}$(tput sgr0)\n\n"
}

function do_curl() {
    curl --silent --fail "${@}"
}

function create_session() {
    do_curl -X GET "$HOST/api/session"
}

function upload_file() {
    local session_key="$1"
    local filename="$2"

    do_curl -X POST --cookie "SESSION_KEY=$session_key" -F "file=@$filename" "$HOST/api/upload"
}

function fetch_file() {
    local url="$1"

    do_curl -X GET -O -J -R "$url"
}

function run_tests() {
    local session_key
    local file_url

    log_test "creating session"
    if ! session_key=$(create_session); then
        log_error "failed to create session"
        exit 1
    fi
    log_success "session created '$session_key'"

    log_test "uploading file '$SCRIPT'"
    if ! file_url=$(upload_file "$session_key" "test.sh"); then
        log_error "failed to upload file '$SCRIPT'"
        exit 1
    fi
    log_success "file uploaded at '$file_url'"

    pushd $(mktemp -d) > /dev/null

    log_test "fetching file at '$file_url'"
    if ! fetch_file "$file_url"; then
        log_error "failed to fetch file at '$file_url'"
        exit 1
    fi
    log_success "received file '$PWD/$(basename "$SCRIPT")'"

    log_test "comparing local and fetched files"
    if ! cmp "$SCRIPT" "$(basename "$SCRIPT")"; then
        log_error "local and fetched files differ"
        exit 1
    fi
    log_success "local and fetched files are the same"
    
    popd > /dev/null

    log_test "uploading file '$SCRIPT' with invalid session key"
    if file_url=$(upload_file "invalid-session-key" "test.sh"); then
        log_error "file uploaded at '$file_url'"
        exit 1
    fi
    log_success "failed to upload file '$SCRIPT'"
}

run_tests
