#ifndef MEC_CLIENT_H
#define MEC_CLIENT_H
#include "config.h"
#ifdef ENABLE_MEC
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <time.h>

// MEC Client configuration defines
#define MAX_URL_SIZE 512
#define MAX_RESPONSE_SIZE 32768
#define MAX_JSON_SIZE 2048
#define MAX_HOST_SIZE 128
#define MAX_ID_SIZE 64

// Response status codes
typedef enum {
    MEC_RSC_OK = 200,
    MEC_RSC_CREATED = 201,
    MEC_RSC_BAD_REQUEST = 400,
    MEC_RSC_NOT_FOUND = 404,
    MEC_RSC_DELETE = 204,
    MEC_RSC_INTERNAL_SERVER_ERROR = 500
} MECResponseStatusCode;

// MEC Client state
typedef enum {
    MEC_STATE_STOPPED = 0,
    MEC_STATE_RUNNING = 1,
    MEC_STATE_REGISTERED = 2
} MECClientState;

// MEC Result structure
typedef struct {
    int status_code;
    char *uuid;
    char *response_data;
    size_t data_length;
    char *error_message;
} MECResult;

// MEC Client structure
typedef struct {
    char cse_id[MAX_ID_SIZE];
    char http_address[MAX_HOST_SIZE];
    int  http_port;

    char mec_host[MAX_HOST_SIZE];
    int  mec_port;
    char mec_protocol[16];
    char mec_platform[64];
    char mec_sandbox_id[64];
    int  mec_enable;

    char iot_platform_id[MAX_ID_SIZE];
    MECClientState state;

    int    registration_attempts;
    time_t last_registration_time;
} MECClient;

// Global MEC client instance
extern MECClient g_mec_client;

// Lifecycle
int  mec_client_init(void);
void mec_client_destroy(void);

// Registration (synchronous, no threads)
MECResult *mec_client_register(void);
MECResult *mec_client_deregister(void);
int        mec_client_is_registered(void);

// Utility
void mec_generate_uuid(char *uuid_str);
void mec_result_destroy(MECResult *result);

#endif /* ENABLE_MEC */
#endif /* MEC_CLIENT_H */
