#ifndef TEST_H
#define TEST_H

#include <stdio.h>
#include <string.h>

#define ASSERT_TRUE(condition) do { \
    if (!(condition)) { \
        fprintf(stderr, "%s:%d: assertion failed: %s\n", __FILE__, __LINE__, #condition); \
        return 1; \
    } \
} while (0)

#define ASSERT_EQ(expected, actual) do { \
    long long test_expected = (long long)(expected); \
    long long test_actual = (long long)(actual); \
    if (test_expected != test_actual) { \
        fprintf(stderr, "%s:%d: expected %lld, got %lld\n", __FILE__, __LINE__, test_expected, test_actual); \
        return 1; \
    } \
} while (0)

#define ASSERT_STR_EQ(expected, actual) do { \
    const char *test_expected = (expected); \
    const char *test_actual = (actual); \
    if (test_expected == NULL || test_actual == NULL || strcmp(test_expected, test_actual) != 0) { \
        fprintf(stderr, "%s:%d: expected \"%s\", got \"%s\"\n", __FILE__, __LINE__, test_expected ? test_expected : "(null)", test_actual ? test_actual : "(null)"); \
        return 1; \
    } \
} while (0)

#define RUN_TEST(test_function) do { \
    int test_result = (test_function)(); \
    if (test_result != 0) { \
        return test_result; \
    } \
    printf("PASS %s\n", #test_function); \
} while (0)

#endif
