#include "http.h"

#include <ctype.h>
#include <stdio.h>
#include <string.h>

static int header_name_equal(const char *start, size_t length, const char *name)
{
    size_t name_length = strlen(name);
    size_t i;
    if (length != name_length) {
        return 0;
    }
    for (i = 0; i < length; ++i) {
        if (tolower((unsigned char)start[i]) != tolower((unsigned char)name[i])) {
            return 0;
        }
    }
    return 1;
}

static const char *status_reason(int status_code)
{
    switch (status_code) {
    case 200: return "OK";
    case 201: return "Created";
    case 204: return "No Content";
    case 400: return "Bad Request";
    case 404: return "Not Found";
    case 500: return "Internal Server Error";
    default: return "Response";
    }
}

int http_parse_request(const char *buffer, size_t length, http_request_t *request)
{
    size_t position = 0;
    size_t line_end;
    size_t method_end;
    size_t path_end;

    if (buffer == NULL || request == NULL || length == 0) {
        return -1;
    }
    memset(request, 0, sizeof(*request));

    while (position + 1 < length && !(buffer[position] == '\r' && buffer[position + 1] == '\n')) {
        ++position;
    }
    if (position + 1 >= length) {
        return -1;
    }
    line_end = position;
    method_end = 0;
    while (method_end < line_end && buffer[method_end] != ' ') {
        ++method_end;
    }
    if (method_end == 0 || method_end >= line_end || method_end >= HTTP_METHOD_MAX) {
        return -1;
    }
    path_end = method_end + 1;
    while (path_end < line_end && buffer[path_end] != ' ') {
        ++path_end;
    }
    if (path_end == method_end + 1 || path_end >= line_end || path_end - method_end - 1 >= HTTP_PATH_MAX) {
        return -1;
    }
    if (line_end - path_end - 1 < 8 || memcmp(buffer + path_end + 1, "HTTP/1.1", 8) != 0) {
        return -1;
    }
    memcpy(request->method, buffer, method_end);
    request->method[method_end] = '\0';
    memcpy(request->path, buffer + method_end + 1, path_end - method_end - 1);
    request->path[path_end - method_end - 1] = '\0';

    position += 2;
    while (position + 1 < length) {
        size_t colon;
        size_t value_start;
        size_t value_end;
        size_t header_end = position;
        while (header_end + 1 < length && !(buffer[header_end] == '\r' && buffer[header_end + 1] == '\n')) {
            ++header_end;
        }
        if (header_end + 1 >= length) {
            return -1;
        }
        if (header_end == position) {
            return 0;
        }
        colon = position;
        while (colon < header_end && buffer[colon] != ':') {
            ++colon;
        }
        if (colon == header_end) {
            return -1;
        }
        value_start = colon + 1;
        while (value_start < header_end && (buffer[value_start] == ' ' || buffer[value_start] == '\t')) {
            ++value_start;
        }
        value_end = header_end;
        while (value_end > value_start && (buffer[value_end - 1] == ' ' || buffer[value_end - 1] == '\t')) {
            --value_end;
        }
        if (header_name_equal(buffer + position, colon - position, "Content-Length")) {
            size_t value = 0;
            size_t i;
            if (value_start == value_end) {
                return -1;
            }
            for (i = value_start; i < value_end; ++i) {
                if (!isdigit((unsigned char)buffer[i])) {
                    return -1;
                }
                if (value > (size_t)-1 / 10) {
                    return -1;
                }
                value = value * 10 + (size_t)(buffer[i] - '0');
            }
            request->content_length = value;
        }
        position = header_end + 2;
    }
    return -1;
}

int http_write_response(char *buffer, size_t capacity, int status_code,
                        const char *content_type, const char *body)
{
    int result;
    size_t body_length;
    if (buffer == NULL || capacity == 0 || body == NULL) {
        return -1;
    }
    body_length = strlen(body);
    if (content_type == NULL) {
        content_type = "text/plain";
    }
    result = snprintf(buffer, capacity,
                      "HTTP/1.1 %d %s\r\nServer: t4\r\nContent-Type: %s\r\nContent-Length: %zu\r\nConnection: close\r\n\r\n%s",
                      status_code, status_reason(status_code), content_type, body_length, body);
    if (result < 0 || (size_t)result >= capacity) {
        return -1;
    }
    return result;
}

int http_response_write(char *buffer, size_t capacity, int status_code,
                        const char *content_type, const char *body)
{
    return http_write_response(buffer, capacity, status_code, content_type, body);
}
