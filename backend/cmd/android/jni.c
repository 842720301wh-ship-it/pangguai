//go:build android && cgo

#include <jni.h>
#include "_cgo_export.h"

JNIEXPORT jint JNICALL Java_com_panguai_account_MainActivity_startBackend(JNIEnv *env, jclass type, jstring directory) {
    const char *value = (*env)->GetStringUTFChars(env, directory, 0);
    if (value == NULL) return 0;
    int port = StartPanguai((char *)value);
    (*env)->ReleaseStringUTFChars(env, directory, value);
    return port;
}
