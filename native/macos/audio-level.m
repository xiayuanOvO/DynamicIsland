#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>
#import <string.h>
#import <unistd.h>

enum { windowSize = 4096 };

static int capture(void) API_AVAILABLE(macos(14.2));

static int capture(void) {
    CATapDescription *tap = [[CATapDescription alloc] initMonoGlobalTapButExcludeProcesses:@[]];
    tap.privateTap = YES;
    tap.muteBehavior = CATapUnmuted;

    AudioObjectID tapID = kAudioObjectUnknown;
    if (AudioHardwareCreateProcessTap(tap, &tapID) != noErr) return 1;

    AudioObjectPropertyAddress formatAddress = {
        kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain
    };
    AudioStreamBasicDescription format = {0};
    UInt32 formatSize = sizeof(format);
    if (AudioObjectGetPropertyData(tapID, &formatAddress, 0, NULL, &formatSize, &format) != noErr ||
        format.mFormatID != kAudioFormatLinearPCM || !(format.mFormatFlags & kAudioFormatFlagIsFloat) ||
        format.mBitsPerChannel != 32 || format.mChannelsPerFrame != 1) {
        AudioHardwareDestroyProcessTap(tapID);
        return 1;
    }

    NSDictionary *configuration = @{
        @kAudioAggregateDeviceNameKey: @"DynamicIsland Audio",
        @kAudioAggregateDeviceUIDKey: NSUUID.UUID.UUIDString,
        @kAudioAggregateDeviceIsPrivateKey: @YES,
        @kAudioAggregateDeviceTapListKey: @[@{
            @kAudioSubTapUIDKey: tap.UUID.UUIDString,
            @kAudioSubTapDriftCompensationKey: @YES
        }]
    };
    AudioObjectID device = kAudioObjectUnknown;
    if (AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)configuration, &device) != noErr) {
        AudioHardwareDestroyProcessTap(tapID);
        return 1;
    }

    AudioDeviceIOProcID proc = NULL;
    dispatch_queue_t queue = dispatch_queue_create("DynamicIsland.AudioLevel", DISPATCH_QUEUE_SERIAL);
    __block struct {
        float ring[windowSize];
        UInt32 index;
        int idleTicks;
    } buffer = {0};
    OSStatus status = AudioDeviceCreateIOProcIDWithBlock(&proc, device, queue,
        ^(const AudioTimeStamp *now, const AudioBufferList *input, const AudioTimeStamp *inputTime,
          AudioBufferList *output, const AudioTimeStamp *outputTime) {
        if (input->mNumberBuffers == 0 || !input->mBuffers[0].mData) return;
        const float *samples = input->mBuffers[0].mData;
        const UInt32 count = input->mBuffers[0].mDataByteSize / sizeof(float);
        for (UInt32 i = 0; i < count; ++i) {
            buffer.ring[buffer.index] = samples[i];
            buffer.index = (buffer.index + 1) % windowSize;
        }
        if (count > 0) buffer.idleTicks = 0;
    });
    if (status == noErr) status = AudioDeviceStart(device, proc);
    if (status == noErr) {
        const UInt32 rate = (UInt32)format.mSampleRate;
        write(STDOUT_FILENO, &rate, sizeof(rate));
        dispatch_source_t timer = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0, queue);
        dispatch_source_set_timer(timer, DISPATCH_TIME_NOW, 32 * NSEC_PER_MSEC, 4 * NSEC_PER_MSEC);
        dispatch_source_set_event_handler(timer, ^{
            if (++buffer.idleTicks > 3) memset(buffer.ring, 0, sizeof(buffer.ring));
            float samples[windowSize];
            memcpy(samples, buffer.ring + buffer.index, (windowSize - buffer.index) * sizeof(float));
            memcpy(samples + windowSize - buffer.index, buffer.ring, buffer.index * sizeof(float));
            const char *bytes = (const char *)samples;
            size_t remaining = sizeof(samples);
            while (remaining > 0) {
                ssize_t sent = write(STDOUT_FILENO, bytes, remaining);
                if (sent <= 0) break;
                bytes += sent;
                remaining -= (size_t)sent;
            }
        });
        dispatch_resume(timer);

        char byte;
        while (read(STDIN_FILENO, &byte, sizeof(byte)) > 0) {}
        dispatch_source_cancel(timer);
        AudioDeviceStop(device, proc);
    }
    if (proc) AudioDeviceDestroyIOProcID(device, proc);
    AudioHardwareDestroyAggregateDevice(device);
    AudioHardwareDestroyProcessTap(tapID);
    return status == noErr ? 0 : 1;
}

int main(void) {
    @autoreleasepool {
        if (@available(macOS 14.2, *)) return capture();
        return 1;
    }
}
