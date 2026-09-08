/*
 * Copyright (c) 2026 DDP Engineering
 * SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
 */
/**
 * @file nilo_sigrok_worker.c
 * @brief Emit bounded structured feasibility records from Sigrok native APIs.
 * @details The executable intentionally has two fixed read/input operations.
 * Native symbols are loaded at runtime so the worker can be built separately
 * from the deployed stack and keep its ABI and embedded Python out of NILO.
 */

#define _POSIX_C_SOURCE 200809L

#include <dlfcn.h>
#include <inttypes.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define SCHEMA "nilo.sigrok.worker/1"
#define SR_OK 0
#define SR_CHANNEL_LOGIC 10000
#define SR_CHANNEL_ANALOG 10001
#define SR_DF_HEADER 10000
#define SR_DF_END 10001
#define SR_DF_META 10002
#define SR_DF_TRIGGER 10003
#define SR_DF_LOGIC 10004
#define SR_DF_FRAME_BEGIN 10005
#define SR_DF_FRAME_END 10006
#define SR_DF_ANALOG 10007
#define SR_KEY_CONFIG 0
#define SR_CONF_GET (1U << 31)
#define SR_CONF_SET (1U << 30)
#define SR_CONF_LIST (1U << 29)
#define SR_CONF_LIMIT_SAMPLES 50001
#define SR_UNIT_VOLT 10000
#define SRD_OUTPUT_ANN 0
#define SRD_OUTPUT_BINARY 2
#define SRD_CONF_SAMPLERATE 10000
#define MAX_RECORDS 200
#define MAX_PACKET_BYTES 256
#define MAX_ANALOG_VALUES 32
#define MAX_DECODER_RECORDS 200

typedef int gboolean;
typedef unsigned int guint;
typedef void *gpointer;
typedef struct GVariant GVariant;
typedef struct GHashTable GHashTable;
typedef struct GSList {
    gpointer data;
    struct GSList *next;
} GSList;
typedef struct GArray {
    char *data;
    guint len;
} GArray;

struct sr_context;
struct sr_session;
struct sr_dev_inst;
struct srd_session;

struct sr_dev_driver {
    const char *name;
    const char *longname;
    int api_version;
};

struct sr_channel {
    struct sr_dev_inst *sdi;
    int index;
    int type;
    gboolean enabled;
    char *name;
    void *priv;
};

struct sr_channel_group {
    char *name;
    GSList *channels;
    void *priv;
};

struct sr_key_info {
    uint32_t key;
    int datatype;
    const char *id;
    const char *name;
    const char *description;
};

struct sr_datafeed_packet {
    uint16_t type;
    const void *payload;
};

struct sr_datafeed_logic {
    uint64_t length;
    uint16_t unitsize;
    void *data;
};

struct sr_rational {
    int64_t p;
    uint64_t q;
};

struct sr_analog_encoding {
    uint8_t unitsize;
    gboolean is_signed;
    gboolean is_float;
    gboolean is_bigendian;
    int8_t digits;
    gboolean is_digits_decimal;
    struct sr_rational scale;
    struct sr_rational offset;
};

struct sr_analog_meaning {
    int mq;
    int unit;
    int mqflags;
    GSList *channels;
};

struct sr_analog_spec {
    int8_t spec_digits;
};

struct sr_datafeed_analog {
    void *data;
    uint32_t num_samples;
    struct sr_analog_encoding *encoding;
    struct sr_analog_meaning *meaning;
    struct sr_analog_spec *spec;
};

struct srd_channel {
    char *id;
    char *name;
    char *desc;
    int order;
};

struct srd_decoder_option {
    char *id;
    char *desc;
    GVariant *def;
    GSList *values;
};

struct srd_decoder {
    char *id;
    char *name;
    char *longname;
    char *desc;
    char *license;
    GSList *inputs;
    GSList *outputs;
    GSList *tags;
    GSList *channels;
    GSList *opt_channels;
    GSList *annotations;
    GSList *annotation_rows;
    GSList *binary;
    GSList *logic_output_channels;
    GSList *options;
    void *py_mod;
    void *py_dec;
};

struct srd_decoder_inst {
    struct srd_decoder *decoder;
    struct srd_session *sess;
    void *py_inst;
    char *inst_id;
};

struct srd_pd_output {
    int pdo_id;
    int output_type;
    struct srd_decoder_inst *di;
    char *proto_id;
};

struct srd_proto_data {
    uint64_t start_sample;
    uint64_t end_sample;
    struct srd_pd_output *pdo;
    void *data;
};

struct srd_proto_data_annotation {
    int ann_class;
    char **ann_text;
};

struct srd_proto_data_binary {
    int bin_class;
    uint64_t size;
    const uint8_t *data;
};

struct runtime_api {
    void *sr_handle;
    void *srd_handle;
    void *glib_handle;
    int (*sr_init)(struct sr_context **);
    int (*sr_exit)(struct sr_context *);
    struct sr_dev_driver **(*sr_driver_list)(const struct sr_context *);
    int (*sr_driver_init)(struct sr_context *, struct sr_dev_driver *);
    GSList *(*sr_driver_scan)(struct sr_dev_driver *, GSList *);
    GArray *(*sr_dev_options)(const struct sr_dev_driver *, const struct sr_dev_inst *,
                             const struct sr_channel_group *);
    int (*sr_dev_config_capabilities_list)(const struct sr_dev_inst *,
                                           const struct sr_channel_group *, int);
    const struct sr_key_info *(*sr_key_info_get)(int, uint32_t);
    int (*config_get)(const struct sr_dev_driver *, const struct sr_dev_inst *,
                      const struct sr_channel_group *, uint32_t, GVariant **);
    int (*config_list)(const struct sr_dev_driver *, const struct sr_dev_inst *,
                       const struct sr_channel_group *, uint32_t, GVariant **);
    int (*config_set_fn)(const struct sr_dev_inst *, const struct sr_channel_group *,
                         uint32_t, GVariant *);
    const char *(*sr_dev_inst_vendor_get)(const struct sr_dev_inst *);
    const char *(*sr_dev_inst_model_get)(const struct sr_dev_inst *);
    const char *(*sr_dev_inst_version_get)(const struct sr_dev_inst *);
    const char *(*sr_dev_inst_sernum_get)(const struct sr_dev_inst *);
    const char *(*sr_dev_inst_connid_get)(const struct sr_dev_inst *);
    GSList *(*sr_dev_inst_channels_get)(const struct sr_dev_inst *);
    GSList *(*sr_dev_inst_channel_groups_get)(const struct sr_dev_inst *);
    int (*sr_dev_open)(struct sr_dev_inst *);
    int (*sr_dev_close)(struct sr_dev_inst *);
    int (*sr_session_new)(struct sr_context *, struct sr_session **);
    int (*sr_session_destroy)(struct sr_session *);
    int (*sr_session_dev_add)(struct sr_session *, struct sr_dev_inst *);
    int (*sr_session_datafeed_callback_add)(struct sr_session *,
        void (*)(const struct sr_dev_inst *, const struct sr_datafeed_packet *, void *), void *);
    int (*sr_session_start)(struct sr_session *);
    int (*sr_session_run)(struct sr_session *);
    int (*sr_analog_to_float)(const struct sr_datafeed_analog *, float *);
    const char *(*sr_package_version_string_get)(void);
    const char *(*sr_lib_version_string_get)(void);
    int (*srd_init)(const char *);
    int (*srd_exit)(void);
    int (*srd_decoder_load_all)(void);
    struct srd_decoder *(*srd_decoder_get_by_id)(const char *);
    int (*srd_session_new)(struct srd_session **);
    int (*srd_session_destroy)(struct srd_session *);
    int (*srd_session_metadata_set)(struct srd_session *, int, GVariant *);
    int (*srd_session_start)(struct srd_session *);
    int (*srd_session_send)(struct srd_session *, uint64_t, uint64_t,
                            const uint8_t *, uint64_t, uint64_t);
    int (*srd_session_send_eof)(struct srd_session *);
    int (*srd_pd_output_callback_add)(struct srd_session *, int,
                                      void (*)(struct srd_proto_data *, void *), void *);
    struct srd_decoder_inst *(*srd_inst_new)(struct srd_session *, const char *, GHashTable *);
    int (*srd_inst_channel_set_all)(struct srd_decoder_inst *, GHashTable *);
    const char *(*srd_package_version_string_get)(void);
    GVariant *(*g_variant_new_uint64)(uint64_t);
    GVariant *(*g_variant_new_int64)(int64_t);
    GVariant *(*g_variant_new_int32)(int32_t);
    const char *(*g_variant_get_type_string)(GVariant *);
    char *(*g_variant_print)(GVariant *, gboolean);
    void (*g_variant_unref)(GVariant *);
    void (*g_free)(gpointer);
    void (*g_array_free)(GArray *, gboolean);
    void (*g_slist_free)(GSList *);
    GHashTable *(*g_hash_table_new)(guint (*)(const void *), gboolean (*)(const void *, const void *));
    gboolean (*g_hash_table_insert)(GHashTable *, gpointer, gpointer);
    void (*g_hash_table_destroy)(GHashTable *);
    guint (*g_str_hash)(const void *);
    gboolean (*g_str_equal)(const void *, const void *);
};

struct capture_state {
    struct runtime_api *api;
    unsigned records;
    uint64_t logic_samples;
    uint64_t analog_samples;
    bool logic;
    bool analog;
    bool trigger;
    bool frame_begin;
    bool frame_end;
    bool truncated;
};

struct decoder_state {
    unsigned records;
    unsigned annotations;
    unsigned binary;
    bool truncated;
};

static void json_string(const char *value)
{
    const unsigned char *cursor = (const unsigned char *)(value ? value : "");
    putchar('"');
    while (*cursor) {
        switch (*cursor) {
        case '"': fputs("\\\"", stdout); break;
        case '\\': fputs("\\\\", stdout); break;
        case '\b': fputs("\\b", stdout); break;
        case '\f': fputs("\\f", stdout); break;
        case '\n': fputs("\\n", stdout); break;
        case '\r': fputs("\\r", stdout); break;
        case '\t': fputs("\\t", stdout); break;
        default:
            if (*cursor < 0x20)
                printf("\\u%04x", *cursor);
            else
                putchar(*cursor);
        }
        cursor++;
    }
    putchar('"');
}

static void emit_error(const char *code, const char *message)
{
    printf("{\"schema\":\"%s\",\"kind\":\"error\",\"code\":", SCHEMA);
    json_string(code);
    fputs(",\"message\":", stdout);
    json_string(message);
    fputs("}\n", stdout);
}

static void *symbol(void *handle, const char *name)
{
    void *value = dlsym(handle, name);
    if (!value) {
        fprintf(stderr, "missing native symbol %s: %s\n", name, dlerror());
        exit(3);
    }
    return value;
}

#define LOAD(api, member, handle, name) (*(void **)(&(api)->member) = symbol((handle), (name)))

static void close_runtime(struct runtime_api *api)
{
    if (api->srd_handle) dlclose(api->srd_handle);
    if (api->sr_handle) dlclose(api->sr_handle);
    if (api->glib_handle) dlclose(api->glib_handle);
}

static int load_runtime(struct runtime_api *api)
{
    const char *directory = getenv("NILO_SIGROK_LIBRARY_DIR");
    char sr_path[1024];
    char srd_path[1024];
    char glib_path[1024];
    memset(api, 0, sizeof(*api));
    if (!directory || !*directory)
        directory = "/opt/pulseview-ddp/usr/lib";
    if (snprintf(sr_path, sizeof(sr_path), "%s/libsigrok.so.4", directory) >= (int)sizeof(sr_path) ||
        snprintf(srd_path, sizeof(srd_path), "%s/libsigrokdecode.so.4", directory) >= (int)sizeof(srd_path) ||
        snprintf(glib_path, sizeof(glib_path), "%s/libglib-2.0.so.0", directory) >= (int)sizeof(glib_path)) {
        emit_error("runtime_path", "Native library directory is too long");
        return 2;
    }
    api->glib_handle = dlopen(glib_path, RTLD_NOW | RTLD_GLOBAL);
    api->sr_handle = dlopen(sr_path, RTLD_NOW | RTLD_LOCAL);
    api->srd_handle = dlopen(srd_path, RTLD_NOW | RTLD_GLOBAL);
    if (!api->glib_handle || !api->sr_handle || !api->srd_handle) {
        emit_error("runtime_unavailable", dlerror());
        close_runtime(api);
        return 2;
    }
    LOAD(api, sr_init, api->sr_handle, "sr_init");
    LOAD(api, sr_exit, api->sr_handle, "sr_exit");
    LOAD(api, sr_driver_list, api->sr_handle, "sr_driver_list");
    LOAD(api, sr_driver_init, api->sr_handle, "sr_driver_init");
    LOAD(api, sr_driver_scan, api->sr_handle, "sr_driver_scan");
    LOAD(api, sr_dev_options, api->sr_handle, "sr_dev_options");
    LOAD(api, sr_dev_config_capabilities_list, api->sr_handle, "sr_dev_config_capabilities_list");
    LOAD(api, sr_key_info_get, api->sr_handle, "sr_key_info_get");
    LOAD(api, config_get, api->sr_handle, "sr_config_get");
    LOAD(api, config_list, api->sr_handle, "sr_config_list");
    LOAD(api, config_set_fn, api->sr_handle, "sr_config_set");
    LOAD(api, sr_dev_inst_vendor_get, api->sr_handle, "sr_dev_inst_vendor_get");
    LOAD(api, sr_dev_inst_model_get, api->sr_handle, "sr_dev_inst_model_get");
    LOAD(api, sr_dev_inst_version_get, api->sr_handle, "sr_dev_inst_version_get");
    LOAD(api, sr_dev_inst_sernum_get, api->sr_handle, "sr_dev_inst_sernum_get");
    LOAD(api, sr_dev_inst_connid_get, api->sr_handle, "sr_dev_inst_connid_get");
    LOAD(api, sr_dev_inst_channels_get, api->sr_handle, "sr_dev_inst_channels_get");
    LOAD(api, sr_dev_inst_channel_groups_get, api->sr_handle, "sr_dev_inst_channel_groups_get");
    LOAD(api, sr_dev_open, api->sr_handle, "sr_dev_open");
    LOAD(api, sr_dev_close, api->sr_handle, "sr_dev_close");
    LOAD(api, sr_session_new, api->sr_handle, "sr_session_new");
    LOAD(api, sr_session_destroy, api->sr_handle, "sr_session_destroy");
    LOAD(api, sr_session_dev_add, api->sr_handle, "sr_session_dev_add");
    LOAD(api, sr_session_datafeed_callback_add, api->sr_handle, "sr_session_datafeed_callback_add");
    LOAD(api, sr_session_start, api->sr_handle, "sr_session_start");
    LOAD(api, sr_session_run, api->sr_handle, "sr_session_run");
    LOAD(api, sr_analog_to_float, api->sr_handle, "sr_analog_to_float");
    LOAD(api, sr_package_version_string_get, api->sr_handle, "sr_package_version_string_get");
    LOAD(api, sr_lib_version_string_get, api->sr_handle, "sr_lib_version_string_get");
    LOAD(api, srd_init, api->srd_handle, "srd_init");
    LOAD(api, srd_exit, api->srd_handle, "srd_exit");
    LOAD(api, srd_decoder_load_all, api->srd_handle, "srd_decoder_load_all");
    LOAD(api, srd_decoder_get_by_id, api->srd_handle, "srd_decoder_get_by_id");
    LOAD(api, srd_session_new, api->srd_handle, "srd_session_new");
    LOAD(api, srd_session_destroy, api->srd_handle, "srd_session_destroy");
    LOAD(api, srd_session_metadata_set, api->srd_handle, "srd_session_metadata_set");
    LOAD(api, srd_session_start, api->srd_handle, "srd_session_start");
    LOAD(api, srd_session_send, api->srd_handle, "srd_session_send");
    LOAD(api, srd_session_send_eof, api->srd_handle, "srd_session_send_eof");
    LOAD(api, srd_pd_output_callback_add, api->srd_handle, "srd_pd_output_callback_add");
    LOAD(api, srd_inst_new, api->srd_handle, "srd_inst_new");
    LOAD(api, srd_inst_channel_set_all, api->srd_handle, "srd_inst_channel_set_all");
    LOAD(api, srd_package_version_string_get, api->srd_handle, "srd_package_version_string_get");
    LOAD(api, g_variant_new_uint64, api->glib_handle, "g_variant_new_uint64");
    LOAD(api, g_variant_new_int64, api->glib_handle, "g_variant_new_int64");
    LOAD(api, g_variant_new_int32, api->glib_handle, "g_variant_new_int32");
    LOAD(api, g_variant_get_type_string, api->glib_handle, "g_variant_get_type_string");
    LOAD(api, g_variant_print, api->glib_handle, "g_variant_print");
    LOAD(api, g_variant_unref, api->glib_handle, "g_variant_unref");
    LOAD(api, g_free, api->glib_handle, "g_free");
    LOAD(api, g_array_free, api->glib_handle, "g_array_free");
    LOAD(api, g_slist_free, api->glib_handle, "g_slist_free");
    LOAD(api, g_hash_table_new, api->glib_handle, "g_hash_table_new");
    LOAD(api, g_hash_table_insert, api->glib_handle, "g_hash_table_insert");
    LOAD(api, g_hash_table_destroy, api->glib_handle, "g_hash_table_destroy");
    LOAD(api, g_str_hash, api->glib_handle, "g_str_hash");
    LOAD(api, g_str_equal, api->glib_handle, "g_str_equal");
    return 0;
}

static bool waveform_setting(const char *id)
{
    static const char *const allowed[] = {
        "samplerate", "limit_samples", "limit_msec", "limit_frames", "continuous",
        "capture_ratio", "averaging", "trigger_match", "timebase", "vdiv", "coupling",
        "num_vdiv", NULL
    };
    for (size_t index = 0; allowed[index]; index++)
        if (strcmp(id, allowed[index]) == 0)
            return true;
    return false;
}

static void emit_variant(struct runtime_api *api, GVariant *value)
{
    char *printed;
    if (!value) {
        fputs("null", stdout);
        return;
    }
    fputs("{\"type\":\"native\",\"native_type\":", stdout);
    json_string(api->g_variant_get_type_string(value));
    fputs(",\"value\":", stdout);
    printed = api->g_variant_print(value, 0);
    json_string(printed);
    api->g_free(printed);
    putchar('}');
}

static void emit_setting(struct runtime_api *api, const struct sr_dev_driver *driver,
                         const struct sr_dev_inst *device, const struct sr_channel_group *group,
                         uint32_t key)
{
    const struct sr_key_info *info = api->sr_key_info_get(SR_KEY_CONFIG, key);
    GVariant *current = NULL;
    GVariant *values = NULL;
    int capabilities;
    if (!info || !info->id || !waveform_setting(info->id))
        return;
    capabilities = api->sr_dev_config_capabilities_list(device, group, (int)key);
    if ((capabilities & SR_CONF_GET) && api->config_get(driver, device, group, key, &current) != SR_OK)
        current = NULL;
    if ((capabilities & SR_CONF_LIST) && api->config_list(driver, device, group, key, &values) != SR_OK)
        values = NULL;
    printf("{\"schema\":\"%s\",\"kind\":\"setting\",\"key\":%" PRIu32 ",\"id\":", SCHEMA, key);
    json_string(info->id);
    fputs(",\"name\":", stdout);
    json_string(info->name);
    fputs(",\"scope\":", stdout);
    json_string(group ? "group" : "device");
    if (group) {
        fputs(",\"group\":", stdout);
        json_string(group->name);
    }
    printf(",\"datatype\":%d,\"get\":%s,\"set\":%s,\"list\":%s,\"current\":",
           info->datatype, capabilities & SR_CONF_GET ? "true" : "false",
           capabilities & SR_CONF_SET ? "true" : "false",
           capabilities & SR_CONF_LIST ? "true" : "false");
    emit_variant(api, current);
    fputs(",\"values\":", stdout);
    emit_variant(api, values);
    fputs("}\n", stdout);
    if (current) api->g_variant_unref(current);
    if (values) api->g_variant_unref(values);
}

static void emit_settings(struct runtime_api *api, const struct sr_dev_driver *driver,
                          const struct sr_dev_inst *device, const struct sr_channel_group *group)
{
    GArray *options = api->sr_dev_options(driver, device, group);
    if (!options)
        return;
    for (guint index = 0; index < options->len; index++) {
        uint32_t key = ((uint32_t *)options->data)[index];
        emit_setting(api, driver, device, group, key);
    }
    api->g_array_free(options, 1);
}

static void emit_event(struct capture_state *state, const char *event)
{
    if (state->records >= MAX_RECORDS) {
        state->truncated = true;
        return;
    }
    printf("{\"schema\":\"%s\",\"kind\":\"event\",\"event\":", SCHEMA);
    json_string(event);
    fputs("}\n", stdout);
    state->records++;
}

static const char *unit_name(int unit)
{
    return unit == SR_UNIT_VOLT ? "volt" : "unknown";
}

static void datafeed(const struct sr_dev_inst *device, const struct sr_datafeed_packet *packet,
                     void *opaque)
{
    struct capture_state *state = opaque;
    (void)device;
    if (packet->type == SR_DF_HEADER) {
        emit_event(state, "header");
    } else if (packet->type == SR_DF_META) {
        emit_event(state, "metadata");
    } else if (packet->type == SR_DF_TRIGGER) {
        state->trigger = true;
        emit_event(state, "trigger");
    } else if (packet->type == SR_DF_FRAME_BEGIN) {
        state->frame_begin = true;
        emit_event(state, "frame_begin");
    } else if (packet->type == SR_DF_FRAME_END) {
        state->frame_end = true;
        emit_event(state, "frame_end");
    } else if (packet->type == SR_DF_END) {
        emit_event(state, "end");
    } else if (packet->type == SR_DF_LOGIC && packet->payload) {
        const struct sr_datafeed_logic *logic = packet->payload;
        const uint8_t *bytes = logic->data;
        uint64_t shown = logic->length < MAX_PACKET_BYTES ? logic->length : MAX_PACKET_BYTES;
        if (!logic->unitsize)
            return;
        if (state->records >= MAX_RECORDS) {
            state->truncated = true;
            return;
        }
        printf("{\"schema\":\"%s\",\"kind\":\"packet\",\"packet\":\"logic\","
               "\"unit_size\":%u,\"byte_count\":\"%" PRIu64 "\",\"sample_count\":\"%" PRIu64 "\",\"data_hex\":\"",
               SCHEMA, logic->unitsize, logic->length, logic->length / logic->unitsize);
        for (uint64_t index = 0; index < shown; index++)
            printf("%02x", bytes[index]);
        printf("\",\"preview_truncated\":%s}\n", shown < logic->length ? "true" : "false");
        state->logic = true;
        state->logic_samples += logic->length / logic->unitsize;
        state->records++;
    } else if (packet->type == SR_DF_ANALOG && packet->payload) {
        const struct sr_datafeed_analog *analog = packet->payload;
        uint32_t shown = analog->num_samples < MAX_ANALOG_VALUES ? analog->num_samples : MAX_ANALOG_VALUES;
        float *values = calloc(analog->num_samples ? analog->num_samples : 1, sizeof(*values));
        if (!values || state->api->sr_analog_to_float(analog, values) != SR_OK) {
            free(values);
            state->truncated = true;
            return;
        }
        if (state->records >= MAX_RECORDS) {
            free(values);
            state->truncated = true;
            return;
        }
        printf("{\"schema\":\"%s\",\"kind\":\"packet\",\"packet\":\"analog\","
               "\"sample_count\":\"%" PRIu32 "\",\"channels\":[", SCHEMA, analog->num_samples);
        bool first = true;
        if (analog->meaning) {
            for (GSList *item = analog->meaning->channels; item; item = item->next) {
                const struct sr_channel *channel = item->data;
                printf("%s%d", first ? "" : ",", channel->index);
                first = false;
            }
        }
        printf("],\"measurement_quantity\":%d,\"unit\":", analog->meaning ? analog->meaning->mq : 0);
        json_string(unit_name(analog->meaning ? analog->meaning->unit : 0));
        printf(",\"measurement_flags\":%d,\"values\":[", analog->meaning ? analog->meaning->mqflags : 0);
        for (uint32_t index = 0; index < shown; index++)
            printf("%s%.9g", index ? "," : "", values[index]);
        printf("],\"preview_truncated\":%s}\n", shown < analog->num_samples ? "true" : "false");
        state->analog = true;
        state->analog_samples += analog->num_samples;
        state->records++;
        free(values);
    }
}

static int demo_capture(struct runtime_api *api, uint64_t samples)
{
    struct sr_context *context = NULL;
    struct sr_session *session = NULL;
    struct sr_dev_driver *driver = NULL;
    struct sr_dev_inst *device;
    GSList *devices = NULL;
    struct capture_state state = {.api = api};
    bool opened = false;
    int result = 4;
    if (api->sr_init(&context) != SR_OK) {
        emit_error("native_init", "libsigrok initialization failed");
        return 4;
    }
    for (struct sr_dev_driver **item = api->sr_driver_list(context); item && *item; item++)
        if ((*item)->name && strcmp((*item)->name, "demo") == 0)
            driver = *item;
    if (!driver || api->sr_driver_init(context, driver) != SR_OK || !(devices = api->sr_driver_scan(driver, NULL))) {
        emit_error("demo_unavailable", "Demo driver could not be scanned");
        goto cleanup;
    }
    device = devices->data;
    printf("{\"schema\":\"%s\",\"kind\":\"runtime\",\"libsigrok\":", SCHEMA);
    json_string(api->sr_package_version_string_get());
    fputs(",\"libsigrok_abi\":", stdout);
    json_string(api->sr_lib_version_string_get());
    fputs(",\"libsigrokdecode\":", stdout);
    json_string(api->srd_package_version_string_get());
    fputs("}\n", stdout);
    printf("{\"schema\":\"%s\",\"kind\":\"driver\",\"id\":", SCHEMA);
    json_string(driver->name);
    fputs(",\"name\":", stdout);
    json_string(driver->longname);
    printf(",\"api_version\":%d}\n", driver->api_version);
    printf("{\"schema\":\"%s\",\"kind\":\"device\",\"vendor\":", SCHEMA);
    json_string(api->sr_dev_inst_vendor_get(device));
    fputs(",\"model\":", stdout); json_string(api->sr_dev_inst_model_get(device));
    fputs(",\"version\":", stdout); json_string(api->sr_dev_inst_version_get(device));
    fputs(",\"serial\":", stdout); json_string(api->sr_dev_inst_sernum_get(device));
    fputs(",\"connection\":", stdout); json_string(api->sr_dev_inst_connid_get(device));
    fputs("}\n", stdout);
    for (GSList *item = api->sr_dev_inst_channels_get(device); item; item = item->next) {
        const struct sr_channel *channel = item->data;
        if (!channel->name)
            continue;
        printf("{\"schema\":\"%s\",\"kind\":\"channel\",\"index\":%d,\"name\":", SCHEMA, channel->index);
        json_string(channel->name);
        fputs(",\"type\":", stdout);
        json_string(channel->type == SR_CHANNEL_LOGIC ? "logic" :
                    channel->type == SR_CHANNEL_ANALOG ? "analog" : "unknown");
        printf(",\"enabled\":%s}\n", channel->enabled ? "true" : "false");
    }
    for (GSList *item = api->sr_dev_inst_channel_groups_get(device); item; item = item->next) {
        const struct sr_channel_group *group = item->data;
        bool first = true;
        printf("{\"schema\":\"%s\",\"kind\":\"channel_group\",\"name\":", SCHEMA);
        json_string(group->name);
        fputs(",\"channels\":[", stdout);
        for (GSList *channel_item = group->channels; channel_item; channel_item = channel_item->next) {
            const struct sr_channel *channel = channel_item->data;
            printf("%s%d", first ? "" : ",", channel->index);
            first = false;
        }
        fputs("]}\n", stdout);
        emit_settings(api, driver, device, group);
    }
    emit_settings(api, driver, device, NULL);
    if (api->sr_dev_open(device) != SR_OK) {
        emit_error("demo_open", "Demo device could not be opened");
        goto cleanup;
    }
    opened = true;
    if (api->config_set_fn(device, NULL, SR_CONF_LIMIT_SAMPLES,
                           api->g_variant_new_uint64(samples)) != SR_OK ||
        api->sr_session_new(context, &session) != SR_OK ||
        api->sr_session_dev_add(session, device) != SR_OK ||
        api->sr_session_datafeed_callback_add(session, datafeed, &state) != SR_OK ||
        api->sr_session_start(session) != SR_OK ||
        api->sr_session_run(session) != SR_OK) {
        emit_error("demo_capture", "Bounded Demo acquisition failed");
        goto cleanup;
    }
    api->sr_dev_close(device);
    opened = false;
    printf("{\"schema\":\"%s\",\"kind\":\"summary\",\"records\":%u,"
           "\"logic\":%s,\"analog\":%s,\"trigger\":%s,\"frame_begin\":%s,"
           "\"frame_end\":%s,\"logic_samples\":\"%" PRIu64 "\","
           "\"analog_samples\":\"%" PRIu64 "\",\"truncated\":%s}\n",
           SCHEMA, state.records, state.logic ? "true" : "false", state.analog ? "true" : "false",
           state.trigger ? "true" : "false", state.frame_begin ? "true" : "false",
           state.frame_end ? "true" : "false", state.logic_samples, state.analog_samples,
           state.truncated ? "true" : "false");
    result = 0;
cleanup:
    if (opened) api->sr_dev_close(device);
    if (session) api->sr_session_destroy(session);
    if (devices) api->g_slist_free(devices);
    api->sr_exit(context);
    return result;
}

static void decoder_output(struct srd_proto_data *data, void *opaque)
{
    struct decoder_state *state = opaque;
    const char *decoder = data->pdo && data->pdo->di && data->pdo->di->decoder ?
        data->pdo->di->decoder->id : "unknown";
    const char *instance = data->pdo && data->pdo->di ? data->pdo->di->inst_id : "unknown";
    if (state->records >= MAX_DECODER_RECORDS) {
        state->truncated = true;
        return;
    }
    if (data->pdo->output_type == SRD_OUTPUT_ANN) {
        const struct srd_proto_data_annotation *annotation = data->data;
        printf("{\"schema\":\"%s\",\"kind\":\"annotation\",\"decoder\":", SCHEMA);
        json_string(decoder); fputs(",\"instance\":", stdout); json_string(instance);
        printf(",\"start_sample\":\"%" PRIu64 "\",\"end_sample\":\"%" PRIu64 "\",\"class\":%d,\"texts\":[",
               data->start_sample, data->end_sample, annotation->ann_class);
        if (annotation->ann_text)
            for (size_t index = 0; annotation->ann_text[index] && index < 8; index++) {
                if (index) putchar(',');
                json_string(annotation->ann_text[index]);
            }
        fputs("]}\n", stdout);
        state->annotations++;
        state->records++;
    } else if (data->pdo->output_type == SRD_OUTPUT_BINARY) {
        const struct srd_proto_data_binary *binary = data->data;
        uint64_t shown = binary->size < MAX_PACKET_BYTES ? binary->size : MAX_PACKET_BYTES;
        printf("{\"schema\":\"%s\",\"kind\":\"binary\",\"decoder\":", SCHEMA);
        json_string(decoder); fputs(",\"instance\":", stdout); json_string(instance);
        printf(",\"start_sample\":\"%" PRIu64 "\",\"end_sample\":\"%" PRIu64 "\","
               "\"class\":%d,\"byte_count\":\"%" PRIu64 "\",\"data_hex\":\"",
               data->start_sample, data->end_sample, binary->bin_class, binary->size);
        for (uint64_t index = 0; index < shown; index++) printf("%02x", binary->data[index]);
        printf("\",\"preview_truncated\":%s}\n", shown < binary->size ? "true" : "false");
        state->binary++;
        state->records++;
    }
}

static void emit_decoder_catalog(struct runtime_api *api, struct srd_decoder *decoder)
{
    printf("{\"schema\":\"%s\",\"kind\":\"decoder\",\"section\":\"identity\",\"id\":", SCHEMA);
    json_string(decoder->id); fputs(",\"name\":", stdout); json_string(decoder->name);
    fputs(",\"long_name\":", stdout); json_string(decoder->longname); fputs("}\n", stdout);
    for (GSList *item = decoder->channels; item; item = item->next) {
        const struct srd_channel *channel = item->data;
        printf("{\"schema\":\"%s\",\"kind\":\"decoder\",\"section\":\"channel\","
               "\"decoder\":\"uart\",\"required\":true,\"id\":", SCHEMA);
        json_string(channel->id); fputs(",\"name\":", stdout); json_string(channel->name); fputs("}\n", stdout);
    }
    for (GSList *item = decoder->opt_channels; item; item = item->next) {
        const struct srd_channel *channel = item->data;
        printf("{\"schema\":\"%s\",\"kind\":\"decoder\",\"section\":\"channel\","
               "\"decoder\":\"uart\",\"required\":false,\"id\":", SCHEMA);
        json_string(channel->id); fputs(",\"name\":", stdout); json_string(channel->name); fputs("}\n", stdout);
    }
    for (GSList *item = decoder->options; item; item = item->next) {
        const struct srd_decoder_option *option = item->data;
        printf("{\"schema\":\"%s\",\"kind\":\"decoder\",\"section\":\"option\","
               "\"decoder\":\"uart\",\"id\":", SCHEMA);
        json_string(option->id); fputs(",\"description\":", stdout); json_string(option->desc);
        fputs(",\"default\":", stdout); emit_variant(api, option->def); fputs("}\n", stdout);
    }
}

static size_t uart_waveform(uint8_t *samples, size_t capacity)
{
    const unsigned samples_per_bit = 10;
    const uint8_t byte = 0x55;
    size_t count = 0;
#define APPEND_LEVEL(level, amount) do { \
    for (unsigned repeat = 0; repeat < (amount) && count < capacity; repeat++) \
        samples[count++] = (level); \
} while (0)
    APPEND_LEVEL(1, 20);
    APPEND_LEVEL(0, samples_per_bit);
    for (unsigned bit = 0; bit < 8; bit++)
        APPEND_LEVEL((byte >> bit) & 1U, samples_per_bit);
    APPEND_LEVEL(1, samples_per_bit);
    APPEND_LEVEL(1, 30);
#undef APPEND_LEVEL
    return count;
}

static int uart_fixture(struct runtime_api *api)
{
    const char *decoder_directory = getenv("NILO_SIGROK_DECODER_DIR");
    struct srd_session *session = NULL;
    struct srd_decoder *decoder;
    struct srd_decoder_inst *instance;
    GHashTable *options = NULL;
    GHashTable *channels = NULL;
    struct decoder_state state = {0};
    uint8_t samples[160];
    size_t count;
    int result = 5;
    if (!decoder_directory || !*decoder_directory)
        decoder_directory = "/opt/pulseview-ddp/usr/share/libsigrokdecode/decoders";
    if (api->srd_init(decoder_directory) != SR_OK || api->srd_decoder_load_all() != SR_OK ||
        !(decoder = api->srd_decoder_get_by_id("uart"))) {
        emit_error("decoder_unavailable", "UART decoder could not be loaded");
        return 5;
    }
    printf("{\"schema\":\"%s\",\"kind\":\"runtime\",\"libsigrok\":", SCHEMA);
    json_string(api->sr_package_version_string_get());
    fputs(",\"libsigrok_abi\":", stdout); json_string(api->sr_lib_version_string_get());
    fputs(",\"libsigrokdecode\":", stdout); json_string(api->srd_package_version_string_get());
    fputs("}\n", stdout);
    emit_decoder_catalog(api, decoder);
    if (api->srd_session_new(&session) != SR_OK)
        goto cleanup;
    options = api->g_hash_table_new(api->g_str_hash, api->g_str_equal);
    channels = api->g_hash_table_new(api->g_str_hash, api->g_str_equal);
    api->g_hash_table_insert(options, "baudrate", api->g_variant_new_int64(100000));
    instance = api->srd_inst_new(session, "uart", options);
    if (!instance)
        goto cleanup;
    api->g_hash_table_insert(channels, "rx", api->g_variant_new_int32(0));
    if (api->srd_inst_channel_set_all(instance, channels) != SR_OK ||
        api->srd_pd_output_callback_add(session, SRD_OUTPUT_ANN, decoder_output, &state) != SR_OK ||
        api->srd_pd_output_callback_add(session, SRD_OUTPUT_BINARY, decoder_output, &state) != SR_OK ||
        api->srd_session_metadata_set(session, SRD_CONF_SAMPLERATE,
                                      api->g_variant_new_uint64(1000000)) != SR_OK ||
        api->srd_session_start(session) != SR_OK)
        goto cleanup;
    count = uart_waveform(samples, sizeof(samples));
    if (api->srd_session_send(session, 0, count - 1, samples, count, 1) != SR_OK ||
        api->srd_session_send_eof(session) != SR_OK)
        goto cleanup;
    printf("{\"schema\":\"%s\",\"kind\":\"summary\",\"records\":%u,"
           "\"annotations\":%u,\"binary_records\":%u,\"logic\":true,"
           "\"analog\":false,\"trigger\":false,\"frame_begin\":false,"
           "\"frame_end\":false,\"truncated\":%s}\n", SCHEMA, state.records,
           state.annotations, state.binary, state.truncated ? "true" : "false");
    result = state.annotations ? 0 : 5;
cleanup:
    if (result != 0)
        emit_error("decoder_fixture", "Deterministic UART decoding failed");
    if (channels) api->g_hash_table_destroy(channels);
    if (options) api->g_hash_table_destroy(options);
    if (session) api->srd_session_destroy(session);
    api->srd_exit();
    return result;
}

static int parse_samples(const char *value, uint64_t *samples)
{
    char *end = NULL;
    unsigned long long parsed = strtoull(value, &end, 10);
    if (!value[0] || !end || *end || parsed < 1 || parsed > 4096)
        return 0;
    *samples = (uint64_t)parsed;
    return 1;
}

int main(int argc, char **argv)
{
    struct runtime_api api;
    uint64_t samples = 64;
    int result;
    if (argc < 2 || (strcmp(argv[1], "demo-capture") != 0 && strcmp(argv[1], "uart-fixture") != 0)) {
        emit_error("invalid_command", "Expected a fixed feasibility operation");
        return 2;
    }
    if (strcmp(argv[1], "demo-capture") == 0) {
        if (argc != 4 || strcmp(argv[2], "--samples") != 0 || !parse_samples(argv[3], &samples)) {
            emit_error("invalid_parameters", "Demo sample count must be between 1 and 4096");
            return 2;
        }
    } else if (argc != 2) {
        emit_error("invalid_parameters", "UART fixture accepts no parameters");
        return 2;
    }
    result = load_runtime(&api);
    if (result)
        return result;
    result = strcmp(argv[1], "demo-capture") == 0 ? demo_capture(&api, samples) : uart_fixture(&api);
    close_runtime(&api);
    return result;
}
