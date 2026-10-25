#include "http.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char **argv)
{
    int port = 8080;
    const char *host = "0.0.0.0";
    const char *data_dir = "./data";

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--port") == 0 && i + 1 < argc) {
            port = atoi(argv[++i]);
            if (port <= 0 || port > 65535) {
                fprintf(stderr, "t4: invalid port\n");
                return 1;
            }
        } else if (strcmp(argv[i], "--host") == 0 && i + 1 < argc) {
            host = argv[++i];
        } else if (strcmp(argv[i], "--data-dir") == 0 && i + 1 < argc) {
            data_dir = argv[++i];
        }
    }

    printf("t4: listening on %s:%d (data directory: %s)\n", host, port, data_dir);
    return 0;
}
