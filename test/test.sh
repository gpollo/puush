#!/usr/bin/bash

HOST="http://127.0.0.1:8080"
SCRIPT="$PWD/$(basename "$0")"

function log_test() {
    local text="$1"

    printf "test: $(tput setaf 3)${text}$(tput sgr0)\n" 1>&2
}

function log_error() {
    local text="$1"

    printf "fail: $(tput setaf 1)${text}$(tput sgr0)\n" 1>&2
}

function log_success() {
    local text="$1"

    printf "ok: $(tput setaf 2)${text}$(tput sgr0)\n\n" 1>&2
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

function delete_file() {
    local session_key="$1"
    local file_url="$2"

    do_curl -X DELETE --cookie "SESSION_KEY=$session_key" "$file_url"
}

function list_files() {
    local session_key="$1"

    do_curl -X GET --cookie "SESSION_KEY=$session_key" "$HOST/api/list"
}

function create_dummy_file() {
    local filename

    filename=$(mktemp)
    cat /dev/urandom | tr -cd '[:alnum:]._-' | head -c 32 > "$filename"

    printf "%s" "$filename"
}

function test_create_session() {
    local session_key

    log_test "creating session"
    if ! session_key=$(create_session); then
        log_error "failed to create session"
        exit 1
    fi
    log_success "session created '$session_key'"

    printf "%s" "$session_key"
}

function test_upload_and_download_file() {
    local session_key="$1"
    local file_name="$2"
    local file_url

    log_test "uploading file '$file_name'"
    if ! file_url=$(upload_file "$session_key" "$file_name"); then
        log_error "failed to upload file '$file_name'"
        exit 1
    fi
    log_success "file uploaded at '$file_url'"

    log_test "fetching file at '$file_url'"
    if ! fetch_file "$file_url"; then
        log_error "failed to fetch file at '$file_url'"
        exit 1
    fi
    log_success "received file '$PWD/$(basename "$file_name")'"

    log_test "comparing local and fetched files"
    if ! cmp "$file_name" "$(basename "$file_name")"; then
        log_error "local and fetched files differ"
        exit 1
    fi
    log_success "local and fetched files are the same"

    printf "%s" "$file_url"
}

function test_upload_file_with_invalid_session_key() {
    local session_key="$1"
    local file_name="$2"
    local file_url

    log_test "uploading file '$file_name' with invalid session key"
    if file_url=$(upload_file "$session_key" "$file_name"); then
        log_error "file uploaded at '$file_url'"
        exit 1
    fi
    log_success "failed to upload file '$file_name'"
}

function test_delete_and_try_fetch_file() {
    local session_key="$1"
    local file_url="$2"

    log_test "deleting file at '$file_url'"
    if ! delete_file "$session_key" "$file_url"; then
        log_error "failed to delete file at '$file_url'"
        exit 1
    fi
    log_success "deleted file at '$file_url'"

    log_test "fetching deleted file at '$file_url'"
    if fetch_file "$file_url"; then
        log_error "fetched deleted file at '$file_url'"
        exit 1
    fi
    log_success "failed to fetch deleted file at '$file_url'"
}

function test_delete_file_with_invalid_session_key() {
    local session_key="$1"
    local file_url="$2"

    log_test "deleting file at '$file_url' with invalid session key"
    if delete_file "$session_key" "$file_url"; then
        log_error "deleted file at '$file_url'"
        exit 1
    fi
    log_success "failed to delete file at '$file_url'"

    log_test "fetching file at '$file_url'"
    if ! fetch_file "$file_url"; then
        log_success "failed to fetch file at '$file_url'"
        exit 1
    fi
    log_error "fetched file at '$file_url'"
}

function test_list_files() {
    local session_key="$1"
    local file_list

    log_test "listing all files for session '$session_key'"
    if ! file_list=$(list_files "$session_key"); then
        log_error "failed to list files"
        exit 1
    fi
    log_success "got file list"

    printf "%s" "$file_list"    
}

function test_list_file_with_invalid_session_key() {
    local session_key="$1"
    local file_list

    log_test "listing all files for invalid session key"
    if file_list=$(list_files "$session_key"); then
        log_error "got file list"
        exit 1
    fi
    log_success "failed to list files"
}

function test_check_file_data_in_file_list() {
    local file_list="$1"
    local file_index="$2"
    local file_id_got
    local file_id_expected="\"$3\""
    local file_name_got
    local file_name_expected="$4"

    file_id_got=$(echo "$file_list" | jq ".[$file_index].id")
    file_name_got=$(echo "$file_list" | jq ".[$file_index].name")
    file_name_expected="\"$(basename "$file_name_expected")\""

    log_test "checking file ID at position $file_index"
    if [[ "$file_id_expected" != "$file_id_got" ]]; then
        log_error "expected $file_id_expected differs from $file_id_got at index $file_index"
        exit 1
    fi
    log_success "$file_id_expected found in file listing at index $file_index"

    log_test "checking file name at position $file_index"
    if [[ "$file_name_expected" != "$file_name_got" ]]; then
        log_error "expected $file_name_expected differs from $file_name_got at index $file_index"
        exit 1
    fi
    log_success "$file_name_expected found in file listing at index $file_index"
}

function run_tests() {
    local session_key
    local file_name_1
    local file_name_2
    local file_name_3
    local file_url_1
    local file_url_2
    local file_url_3
    local file_id_1
    local file_id_2
    local file_id_3

    file_name_1=$(readlink -f test.sh)
    file_name_2=$(create_dummy_file)
    file_name_3=$(create_dummy_file)

    pushd $(mktemp -d) > /dev/null

    session_key=$(test_create_session)

    file_url_1=$(test_upload_and_download_file "$session_key" "$file_name_1")
    file_url_2=$(test_upload_and_download_file "$session_key" "$file_name_2")
    file_url_3=$(test_upload_and_download_file "$session_key" "$file_name_3")

    file_id_1=$(basename "$file_url_1" | sed 's/\..*//g')
    file_id_2=$(basename "$file_url_2" | sed 's/\..*//g')
    file_id_3=$(basename "$file_url_3" | sed 's/\..*//g')

    file_list=$(test_list_files "$session_key")

    test_check_file_data_in_file_list "$file_list" 0 "$file_id_1" "$file_name_1"
    test_check_file_data_in_file_list "$file_list" 1 "$file_id_2" "$file_name_2"
    test_check_file_data_in_file_list "$file_list" 2 "$file_id_3" "$file_name_3"

    test_delete_and_try_fetch_file "$session_key" "$file_url_1"
    test_delete_and_try_fetch_file "$session_key" "$file_url_2"

    test_upload_file_with_invalid_session_key "invalid-session-key" "$file_name_1"
    test_list_file_with_invalid_session_key "invalid-session-key"
    test_delete_file_with_invalid_session_key "invalid-session-key" "$file_name_3"

    popd > /dev/null
}

run_tests
