#include "../../src/http.h"
#include "test.h"

#include <string.h>

static int test_parse_request_with_content_length(void)
{
    static const char request_text[] =
        "POST /submit?q=1 HTTP/1.1\r\n"
        "Host: example.test\r\n"
        "Content-Length: 12\r\n"
        "Connection: close\r\n\r\n"
        "hello world!";
    http_request_t request;

    ASSERT_EQ(0, http_parse_request(request_text, strlen(request_text), &request));
    ASSERT_STR_EQ("POST", request.method);
    ASSERT_STR_EQ("/submit?q=1", request.path);
    ASSERT_EQ(12, request.content_length);
    return 0;
}

static int test_parse_request_is_case_insensitive_and_trims_value(void)
{
    static const char request_text[] =
        "GET / HTTP/1.1\r\n"
        "content-length:\t7  \r\n\r\n";
    http_request_t request;

    ASSERT_EQ(0, http_parse_request(request_text, strlen(request_text), &request));
    ASSERT_STR_EQ("GET", request.method);
    ASSERT_STR_EQ("/", request.path);
    ASSERT_EQ(7, request.content_length);
    return 0;
}

static int test_parse_request_rejects_invalid_input(void)
{
    static const char missing_terminator[] = "GET / HTTP/1.1\r\nHost: x\r\n";
    static const char bad_length[] = "POST / HTTP/1.1\r\nContent-Length: abc\r\n\r\n";
    static const char bad_version[] = "GET / HTTP/1.0\r\n\r\n";
    http_request_t request;

    ASSERT_EQ(-1, http_parse_request(NULL, 0, &request));
    ASSERT_EQ(-1, http_parse_request(missing_terminator, strlen(missing_terminator), &request));
    ASSERT_EQ(-1, http_parse_request(bad_length, strlen(bad_length), &request));
    ASSERT_EQ(-1, http_parse_request(bad_version, strlen(bad_version), &request));
    ASSERT_EQ(-1, http_parse_request("", 0, NULL));
    return 0;
}

static int test_write_response_includes_wire_headers(void)
{
    char response[256];
    int written = http_write_response(response, sizeof(response), 201, "text/plain", "created");

    ASSERT_TRUE(written > 0);
    ASSERT_STR_EQ(
        "HTTP/1.1 201 Created\r\n"
        "Server: t4\r\n"
        "Content-Type: text/plain\r\n"
        "Content-Length: 7\r\n"
        "Connection: close\r\n\r\n"
        "created", response);
    ASSERT_EQ((int)strlen(response), written);
    return 0;
}

static int test_response_defaults_and_alias(void)
{
    char response[256];
    int written = http_response_write(response, sizeof(response), 404, NULL, "missing");

    ASSERT_TRUE(written > 0);
    ASSERT_TRUE(strstr(response, "HTTP/1.1 404 Not Found\r\n") == response);
    ASSERT_TRUE(strstr(response, "Server: t4\r\n") != NULL);
    ASSERT_TRUE(strstr(response, "Content-Type: text/plain\r\n") != NULL);
    ASSERT_TRUE(strstr(response, "Content-Length: 7\r\n") != NULL);
    ASSERT_TRUE(strstr(response, "Connection: close\r\n\r\nmissing") != NULL);
    return 0;
}

static int test_write_response_rejects_null_and_small_buffers(void)
{
    char response[8];

    ASSERT_EQ(-1, http_write_response(NULL, sizeof(response), 200, NULL, "ok"));
    ASSERT_EQ(-1, http_write_response(response, sizeof(response), 200, NULL, "ok"));
    ASSERT_EQ(-1, http_write_response(response, sizeof(response), 200, NULL, NULL));
    ASSERT_EQ(-1, http_write_response(response, 0, 200, NULL, "ok"));
    return 0;
}

int main(void)
{
    RUN_TEST(test_parse_request_with_content_length);
    RUN_TEST(test_parse_request_is_case_insensitive_and_trims_value);
    RUN_TEST(test_parse_request_rejects_invalid_input);
    RUN_TEST(test_write_response_includes_wire_headers);
    RUN_TEST(test_response_defaults_and_alias);
    RUN_TEST(test_write_response_rejects_null_and_small_buffers);
    return 0;
}
