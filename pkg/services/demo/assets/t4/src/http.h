#ifndef HTTP_H
#define HTTP_H

#include <stddef.h>

#define HTTP_METHOD_MAX 16
#define HTTP_PATH_MAX 1024

typedef struct {
    char method[HTTP_METHOD_MAX];
    char path[HTTP_PATH_MAX];
    size_t content_length;
} http_request_t;

int http_parse_request(const char *buffer, size_t length, http_request_t *request);
int http_write_response(char *buffer, size_t capacity, int status_code,
                        const char *content_type, const char *body);
int http_response_write(char *buffer, size_t capacity, int status_code,
                        const char *content_type, const char *body);

#endif
