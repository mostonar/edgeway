#include "mecClient.h"
#include "config.h"
#ifdef ENABLE_MEC
#include "logger.h"
#include "cJSON.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <time.h>
#include <sys/socket.h>
#include <netdb.h>
#include <arpa/inet.h>
#include <openssl/ssl.h>
#include <openssl/err.h>

// Global MEC client instance
MECClient g_mec_client;

// ─────────────────────────────────────────────
// UUID generation (simple, no libuuid dependency)
// ─────────────────────────────────────────────
void mec_generate_uuid(char *uuid_str) {
    static unsigned int counter = 0;
    srand((unsigned int)time(NULL) + counter++);
    snprintf(uuid_str, 37, "%08x-%04x-4%03x-%04x-%08x%04x",
             (unsigned int)rand(),
             (unsigned int)(rand() & 0xFFFF),
             (unsigned int)(rand() & 0x0FFF),
             (unsigned int)((rand() & 0x3FFF) | 0x8000),
             (unsigned int)rand(),
             (unsigned int)(rand() & 0xFFFF));
}

// ─────────────────────────────────────────────
// Minimal HTTPS POST/DELETE using OpenSSL directly
// Returns HTTP status code, or -1 on connection error
// ─────────────────────────────────────────────
static int mec_https_request(const char *method, const char *host, int port,
                              const char *path, const char *body,
                              char *resp_buf, size_t resp_buf_size) {
    int sockfd = -1;
    SSL_CTX *ctx = NULL;
    SSL *ssl = NULL;
    int status_code = -1;

    // Resolve host
    struct addrinfo hints, *res = NULL;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family   = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;

    char port_str[8];
    snprintf(port_str, sizeof(port_str), "%d", port);

    if (getaddrinfo(host, port_str, &hints, &res) != 0) {
        logger("MEC", LOG_LEVEL_ERROR, "DNS resolution failed for %s", host);
        return -1;
    }

    sockfd = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
    if (sockfd < 0) {
        logger("MEC", LOG_LEVEL_ERROR, "Socket creation failed");
        freeaddrinfo(res);
        return -1;
    }

    // Set socket timeout (10s)
    struct timeval tv = { .tv_sec = 10, .tv_usec = 0 };
    setsockopt(sockfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
    setsockopt(sockfd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));

    if (connect(sockfd, res->ai_addr, res->ai_addrlen) != 0) {
        logger("MEC", LOG_LEVEL_ERROR, "TCP connect failed to %s:%d", host, port);
        freeaddrinfo(res);
        close(sockfd);
        return -1;
    }
    freeaddrinfo(res);

    // TLS setup
    SSL_library_init();
    SSL_load_error_strings();
    ctx = SSL_CTX_new(TLS_client_method());
    if (!ctx) {
        logger("MEC", LOG_LEVEL_ERROR, "SSL_CTX_new failed");
        close(sockfd);
        return -1;
    }
    SSL_CTX_set_verify(ctx, SSL_VERIFY_NONE, NULL);

    ssl = SSL_new(ctx);
    SSL_set_fd(ssl, sockfd);
    SSL_set_tlsext_host_name(ssl, host);

    if (SSL_connect(ssl) <= 0) {
        logger("MEC", LOG_LEVEL_ERROR, "SSL handshake failed");
        goto cleanup;
    }

    // Build HTTP request
    char *req = malloc(4096 + (body ? strlen(body) : 0));
    if (!req) goto cleanup;

    if (body && strlen(body) > 0) {
        snprintf(req, 4096 + strlen(body),
                 "%s %s HTTP/1.1\r\n"
                 "Host: %s\r\n"
                 "Content-Type: application/json; charset=UTF-8\r\n"
                 "Accept: application/json\r\n"
                 "Content-Length: %zu\r\n"
                 "Connection: close\r\n"
                 "\r\n"
                 "%s",
                 method, path, host, strlen(body), body);
    } else {
        snprintf(req, 4096,
                 "%s %s HTTP/1.1\r\n"
                 "Host: %s\r\n"
                 "Accept: application/json\r\n"
                 "Connection: close\r\n"
                 "\r\n",
                 method, path, host);
    }

    SSL_write(ssl, req, strlen(req));
    free(req);

    // Read response
    char *raw = calloc(resp_buf_size, 1);
    if (!raw) goto cleanup;

    int total = 0, n;
    while ((n = SSL_read(ssl, raw + total, (int)(resp_buf_size - total - 1))) > 0) {
        total += n;
    }
    raw[total] = '\0';

    // Parse status code from "HTTP/1.x NNN ..."
    if (sscanf(raw, "HTTP/%*s %d", &status_code) != 1) {
        status_code = -1;
    }

    // Copy body (after \r\n\r\n) into resp_buf
    char *body_start = strstr(raw, "\r\n\r\n");
    if (body_start && resp_buf) {
        body_start += 4;
        strncpy(resp_buf, body_start, resp_buf_size - 1);
        resp_buf[resp_buf_size - 1] = '\0';
    }
    free(raw);

cleanup:
    if (ssl) { SSL_shutdown(ssl); SSL_free(ssl); }
    if (ctx) SSL_CTX_free(ctx);
    if (sockfd >= 0) close(sockfd);
    return status_code;
}

// ─────────────────────────────────────────────
// Init
// ─────────────────────────────────────────────
int mec_client_init(void) {
    memset(&g_mec_client, 0, sizeof(MECClient));

    strncpy(g_mec_client.cse_id,        CSE_BASE_RI,    MAX_ID_SIZE   - 1);
    strncpy(g_mec_client.http_address,  MEC_HTTP_HOST,  MAX_HOST_SIZE - 1);
    g_mec_client.http_port = atoi(SERVER_PORT);

    strncpy(g_mec_client.mec_host,       MEC_HOST,       MAX_HOST_SIZE - 1);
    strncpy(g_mec_client.mec_protocol,   MEC_PROTOCOL,   15);
    strncpy(g_mec_client.mec_platform,   MEC_PLATFORM,   63);
    strncpy(g_mec_client.mec_sandbox_id, MEC_SANDBOX_ID, 63);
    g_mec_client.mec_port = 443;

    g_mec_client.state = MEC_STATE_STOPPED;
    g_mec_client.mec_enable = 1;

    logger("MEC", LOG_LEVEL_INFO,
           "MEC Client config: cse_id=%s, http_address=%s, http_port=%d, "
           "mec_host=%s, mec_protocol=%s, mec_platform=%s, mec_sandbox_id=%s",
           g_mec_client.cse_id, g_mec_client.http_address, g_mec_client.http_port,
           g_mec_client.mec_host, g_mec_client.mec_protocol,
           g_mec_client.mec_platform, g_mec_client.mec_sandbox_id);
    logger("MEC", LOG_LEVEL_INFO, "MEC Client initialized");
    return 1;
}

// ─────────────────────────────────────────────
// Destroy
// ─────────────────────────────────────────────
void mec_client_destroy(void) {
    g_mec_client.state = MEC_STATE_STOPPED;
    logger("MEC", LOG_LEVEL_INFO, "MEC Client destroyed");
}

// ─────────────────────────────────────────────
// Register  (synchronous, called from main thread before serve_forever)
// ─────────────────────────────────────────────
MECResult *mec_client_register(void) {
    char uuid_str1[37], uuid_str2[37], uuid_str3[37];
    mec_generate_uuid(uuid_str1);
    mec_generate_uuid(uuid_str2);
    mec_generate_uuid(uuid_str3);
    strncpy(g_mec_client.iot_platform_id, uuid_str1, MAX_ID_SIZE - 1);

    // Build path
    char path[512];
    snprintf(path, sizeof(path), "/%s/%s/iots/v1/registered_iot_platforms",
             g_mec_client.mec_sandbox_id, g_mec_client.mec_platform);

    // Build JSON body
    cJSON *json = cJSON_CreateObject();
    cJSON_AddStringToObject(json, "iotPlatformId", uuid_str1);

    // userTransportInfo
    cJSON *uti_arr = cJSON_CreateArray();
    cJSON *uti = cJSON_CreateObject();
    cJSON_AddStringToObject(uti, "id", uuid_str2);
    cJSON_AddStringToObject(uti, "name", g_mec_client.cse_id);
    cJSON_AddStringToObject(uti, "description", "MQTT");
    cJSON_AddStringToObject(uti, "type", "MB_TOPIC_BASED");
    cJSON_AddStringToObject(uti, "protocol", "MQTT");
    cJSON_AddStringToObject(uti, "version", "2");
    cJSON *ep1 = cJSON_CreateObject();
    cJSON *addr1 = cJSON_CreateArray();
    cJSON *a1 = cJSON_CreateObject();
    cJSON_AddStringToObject(a1, "host", "meep-mosquitto");
    cJSON_AddNumberToObject(a1, "port", 1883);
    cJSON_AddItemToArray(addr1, a1);
    cJSON_AddItemToObject(ep1, "addresses", addr1);
    cJSON_AddItemToObject(uti, "endpoint", ep1);
    cJSON_AddItemToObject(uti, "security", cJSON_CreateObject());
    cJSON_AddItemToObject(uti, "implSpecificInfo", cJSON_CreateObject());
    cJSON_AddItemToArray(uti_arr, uti);
    cJSON_AddItemToObject(json, "userTransportInfo", uti_arr);

    // customServicesTransportInfo
    cJSON *cst_arr = cJSON_CreateArray();
    cJSON *cst = cJSON_CreateObject();
    cJSON_AddStringToObject(cst, "id", uuid_str3);
    cJSON_AddStringToObject(cst, "name", g_mec_client.cse_id);
    cJSON_AddStringToObject(cst, "description", "TinyIOT oneM2M CSE");
    cJSON_AddStringToObject(cst, "type", "REST_HTTP");
    cJSON_AddStringToObject(cst, "protocol", "REST_HTTP");
    cJSON_AddStringToObject(cst, "version", "4");
    cJSON *ep2 = cJSON_CreateObject();
    cJSON *addr2 = cJSON_CreateArray();
    cJSON *a2 = cJSON_CreateObject();
    cJSON_AddStringToObject(a2, "host", g_mec_client.http_address);
    cJSON_AddNumberToObject(a2, "port", g_mec_client.http_port);
    cJSON_AddItemToArray(addr2, a2);
    cJSON_AddItemToObject(ep2, "addresses", addr2);
    cJSON_AddItemToObject(cst, "endpoint", ep2);
    cJSON_AddItemToObject(cst, "security", cJSON_CreateObject());
    cJSON_AddItemToArray(cst_arr, cst);
    cJSON_AddItemToObject(json, "customServicesTransportInfo", cst_arr);

    cJSON_AddBoolToObject(json, "enabled", 1);

    char *body = cJSON_PrintUnformatted(json);
    cJSON_Delete(json);

    char *resp = calloc(8192, 1);
    MECResult *result = calloc(1, sizeof(MECResult));

    logger("MEC", LOG_LEVEL_INFO, "Registering to MEC platform %s (path: %s)",
           g_mec_client.mec_host, path);

    int status = mec_https_request("POST", g_mec_client.mec_host,
                                   g_mec_client.mec_port, path, body, resp, 8192);
    free(body);

    result->status_code = status;
    result->response_data = resp;
    result->data_length = strlen(resp);
    result->uuid = strdup(uuid_str1);
    result->error_message = NULL;

    if (status == MEC_RSC_CREATED) {
        g_mec_client.state = MEC_STATE_REGISTERED;
        g_mec_client.last_registration_time = time(NULL);
        logger("MEC", LOG_LEVEL_INFO, "MEC registration successful (201)");
    } else {
        logger("MEC", LOG_LEVEL_ERROR, "MEC registration failed: %d", status);
    }

    return result;
}

// ─────────────────────────────────────────────
// Deregister  (synchronous, called from stop_server)
// ─────────────────────────────────────────────
MECResult *mec_client_deregister(void) {
    MECResult *result = calloc(1, sizeof(MECResult));

    if (strlen(g_mec_client.iot_platform_id) == 0) {
        result->status_code = MEC_RSC_INTERNAL_SERVER_ERROR;
        result->error_message = strdup("Not registered");
        return result;
    }

    char path[512];
    snprintf(path, sizeof(path), "/%s/%s/iots/v1/registered_iot_platforms/%s",
             g_mec_client.mec_sandbox_id, g_mec_client.mec_platform,
             g_mec_client.iot_platform_id);

    char *resp = calloc(4096, 1);

    logger("MEC", LOG_LEVEL_INFO, "Deregistering from MEC platform (path: %s)", path);

    int status = mec_https_request("DELETE", g_mec_client.mec_host,
                                   g_mec_client.mec_port, path, NULL, resp, 4096);

    memset(g_mec_client.iot_platform_id, 0, MAX_ID_SIZE);
    g_mec_client.state = MEC_STATE_STOPPED;

    result->status_code = status;
    result->response_data = resp;
    result->data_length = strlen(resp);

    if (status == MEC_RSC_DELETE) {
        logger("MEC", LOG_LEVEL_INFO, "MEC deregistration successful (204)");
    } else {
        logger("MEC", LOG_LEVEL_ERROR, "MEC deregistration failed: %d", status);
    }

    return result;
}

// ─────────────────────────────────────────────
// Result cleanup
// ─────────────────────────────────────────────
void mec_result_destroy(MECResult *result) {
    if (!result) return;
    if (result->response_data) free(result->response_data);
    if (result->error_message) free(result->error_message);
    if (result->uuid)          free(result->uuid);
    free(result);
}

int mec_client_is_registered(void) {
    return g_mec_client.state == MEC_STATE_REGISTERED;
}

#endif /* ENABLE_MEC */
