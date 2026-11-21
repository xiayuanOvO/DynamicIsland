#import <CoreBluetooth/CoreBluetooth.h>
#import <Foundation/Foundation.h>
#import <IOBluetooth/IOBluetooth.h>

static CBUUID *batteryService, *batteryLevel;
static IOBluetoothSDPUUID *encoService;
static NSString *const encoModel = @"OPPO Enco Free4";

@interface BatteryProbe : NSObject <CBCentralManagerDelegate, CBPeripheralDelegate, IOBluetoothRFCOMMChannelDelegate>
@property(nonatomic, strong) CBCentralManager *central;
@property(nonatomic, strong) NSSet<NSString *> *targets;
@property(nonatomic, strong) NSMutableSet<CBPeripheral *> *pending;
@property(nonatomic, strong) NSMutableArray<NSDictionary *> *results;
@property(nonatomic, strong) IOBluetoothRFCOMMChannel *encoChannel;
@property(nonatomic, strong) NSMutableData *encoBuffer;
@property(nonatomic) BOOL encoPending;
@property(nonatomic) BOOL started;
@property(nonatomic, readonly) BOOL done;
@end

@implementation BatteryProbe

- (BOOL)done {
    return self.started && self.pending.count == 0 && !self.encoPending;
}

- (void)centralManagerDidUpdateState:(CBCentralManager *)central {
    if (self.started || central.state == CBManagerStateUnknown || central.state == CBManagerStateResetting) return;
    self.started = YES;
    if (central.state != CBManagerStatePoweredOn) return;

    if ([self.targets containsObject:encoModel]) {
        NSPredicate *connectedEnco = [NSPredicate predicateWithBlock:^BOOL(IOBluetoothDevice *device, NSDictionary *bindings) {
            return device.isConnected && [device.name isEqualToString:encoModel];
        }];
        NSArray<IOBluetoothDevice *> *devices = [IOBluetoothDevice.pairedDevices filteredArrayUsingPredicate:connectedEnco];
        if (devices.count == 1) {
            IOBluetoothDevice *device = devices.firstObject;
            self.encoPending = YES;
            if ([device getServiceRecordForUUID:encoService]) {
                [self sdpQueryComplete:device status:kIOReturnSuccess];
            } else if ([device performSDPQuery:self] != kIOReturnSuccess) {
                self.encoPending = NO;
            }
        }
    }

    NSArray<CBPeripheral *> *connected = [central retrieveConnectedPeripheralsWithServices:@[batteryService]];
    for (NSString *name in self.targets) {
        if ([name isEqualToString:encoModel]) continue;
        CBPeripheral *match = nil;
        for (CBPeripheral *peripheral in connected) {
            if (![peripheral.name isEqualToString:name]) continue;
            if (match != nil) {
                match = nil;
                break;
            }
            match = peripheral;
        }
        if (!match) continue;
        [self.pending addObject:match];
        match.delegate = self;
        [central connectPeripheral:match options:nil];
    }
}

- (void)centralManager:(CBCentralManager *)central didConnectPeripheral:(CBPeripheral *)peripheral {
    [peripheral discoverServices:@[batteryService]];
}

- (void)centralManager:(CBCentralManager *)central didFailToConnectPeripheral:(CBPeripheral *)peripheral error:(NSError *)error {
    [self.pending removeObject:peripheral];
}

- (void)finishPeripheral:(CBPeripheral *)peripheral {
    [self.central cancelPeripheralConnection:peripheral];
    [self.pending removeObject:peripheral];
}

- (void)peripheral:(CBPeripheral *)peripheral didDiscoverServices:(NSError *)error {
    // Multiple Battery Services are separate batteries. A single ring needs
    // an unambiguous reading.
    if (!error && peripheral.services.count == 1) {
        [peripheral discoverCharacteristics:@[batteryLevel] forService:peripheral.services.firstObject];
    } else {
        [self finishPeripheral:peripheral];
    }
}

- (void)peripheral:(CBPeripheral *)peripheral didDiscoverCharacteristicsForService:(CBService *)service error:(NSError *)error {
    CBCharacteristic *level = service.characteristics.firstObject;
    if (!error && (level.properties & CBCharacteristicPropertyRead)) {
        [peripheral readValueForCharacteristic:level];
    } else {
        [self finishPeripheral:peripheral];
    }
}

- (void)peripheral:(CBPeripheral *)peripheral didUpdateValueForCharacteristic:(CBCharacteristic *)characteristic error:(NSError *)error {
    if (!error && characteristic.value && peripheral.name) {
        [self.results addObject:@{@"name": peripheral.name,
                                  @"data": [characteristic.value base64EncodedStringWithOptions:0]}];
    }
    [self finishPeripheral:peripheral];
}

- (void)sdpQueryComplete:(IOBluetoothDevice *)device status:(IOReturn)status {
    IOBluetoothSDPServiceRecord *service = [device getServiceRecordForUUID:encoService];
    BluetoothRFCOMMChannelID channelID = 0;
    IOBluetoothRFCOMMChannel *channel = nil;
    if (status != kIOReturnSuccess || !device.isConnected || !service ||
        [service getRFCOMMChannelID:&channelID] != kIOReturnSuccess ||
        [device openRFCOMMChannelAsync:&channel withChannelID:channelID delegate:self] != kIOReturnSuccess) {
        self.encoPending = NO;
        return;
    }
    self.encoChannel = channel;
}

- (void)rfcommChannelOpenComplete:(IOBluetoothRFCOMMChannel *)channel status:(IOReturn)status {
    // HeyMelody 0x0106 is a battery query. The sequence number is 3.
    uint8_t request[] = {0xaa, 7, 0, 0, 6, 1, 3, 0, 0};
    if (status != kIOReturnSuccess || [channel writeSync:request length:sizeof(request)] != kIOReturnSuccess) {
        self.encoPending = NO;
    }
}

- (void)rfcommChannelData:(IOBluetoothRFCOMMChannel *)channel data:(void *)data length:(size_t)length {
    if (!self.encoPending) return;
    [self.encoBuffer appendBytes:data length:length];
    while (self.encoBuffer.length >= 2) {
        // RFCOMM is a stream. Ignore unsolicited frames, retaining a partial
        // frame until more data arrives.
        const uint8_t *bytes = self.encoBuffer.bytes;
        if (bytes[0] != 0xaa) {
            const uint8_t *start = memchr(bytes, 0xaa, self.encoBuffer.length);
            NSUInteger skip = start ? (NSUInteger)(start - bytes) : self.encoBuffer.length;
            [self.encoBuffer replaceBytesInRange:NSMakeRange(0, skip) withBytes:NULL length:0];
            continue;
        }
        NSUInteger header = (bytes[1] & 0x80) ? 3 : 2;
        if (self.encoBuffer.length < header) return;
        NSUInteger size = header + (bytes[1] & 0x7f);
        if (header == 3) size += (NSUInteger)bytes[2] << 7;
        if (self.encoBuffer.length < size) return;
        if (header == 2 && size >= 9 && bytes[4] == 6 && bytes[5] == 0x81 && bytes[6] == 3) {
            NSData *reply = [NSData dataWithBytes:bytes length:size];
            [self.results addObject:@{@"name": encoModel, @"oppo": @YES, @"data": [reply base64EncodedStringWithOptions:0]}];
            self.encoPending = NO;
            return;
        }
        [self.encoBuffer replaceBytesInRange:NSMakeRange(0, size) withBytes:NULL length:0];
    }
}

- (void)rfcommChannelClosed:(IOBluetoothRFCOMMChannel *)channel {
    self.encoPending = NO;
}

@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSMutableSet *targets = [NSMutableSet set];
        for (int i = 1; i < argc; i++) {
            NSString *name = [NSString stringWithUTF8String:argv[i]];
            if (name.length) [targets addObject:name];
        }
        if (targets.count == 0) return 0;
        batteryService = [CBUUID UUIDWithString:@"180F"];
        batteryLevel = [CBUUID UUIDWithString:@"2A19"];
        // 0000079a-d102-11e1-9b23-00025b00a5a5, resolved over SDP.
        const uint8_t uuid[] = {0, 0, 7, 0x9a, 0xd1, 2, 0x11, 0xe1, 0x9b, 0x23, 0, 2, 0x5b, 0, 0xa5, 0xa5};
        encoService = [IOBluetoothSDPUUID uuidWithBytes:uuid length:sizeof(uuid)];

        BatteryProbe *probe = [BatteryProbe new];
        probe.targets = targets;
        probe.pending = [NSMutableSet set];
        probe.results = [NSMutableArray array];
        probe.encoBuffer = [NSMutableData data];
        probe.central = [[CBCentralManager alloc] initWithDelegate:probe queue:nil options:@{CBCentralManagerOptionShowPowerAlertKey: @NO}];
        NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:2.0];
        while (!probe.done && deadline.timeIntervalSinceNow > 0) {
            [NSRunLoop.currentRunLoop runMode:NSDefaultRunLoopMode beforeDate:deadline];
        }
        for (CBPeripheral *peripheral in probe.pending) {
            [probe.central cancelPeripheralConnection:peripheral];
        }
        [probe.encoChannel closeChannel];
        NSData *json = [NSJSONSerialization dataWithJSONObject:probe.results options:0 error:NULL];
        if (json) fwrite(json.bytes, 1, json.length, stdout);
    }
    return 0;
}
