import { Injectable } from '@angular/core';
import Device from '../device';
import { Observable } from 'rxjs';
import { SystemConfigService } from './system-config.service';
import { SystemConnectionsService } from './system-connections.service';
import { DbCompletionService } from './db-completion.service';

@Injectable({
  providedIn: 'root'
})
export class DeviceService {
  private devices: Device[];
  constructor(
    private systemConfigService: SystemConfigService,
    private systemConnectionsService: SystemConnectionsService,
    private dbCompletionService: DbCompletionService
  ) { }

  getAll(): Observable<Device> {
    const deviceObservable: Observable<Device> = new Observable((observer) => {
      this.systemConfigService.getDevices().subscribe(
        devices => {
          this.devices = devices;

          // Check folder devices to see if the device is used
          this.systemConfigService.getFolders().subscribe(
            folders => {
              // TODO: streamline
              // Loop through all folder devices to see if the device is used
              this.devices.forEach(device => {
                folders.forEach(folder => {
                  folder.devices.forEach(fdevice => {
                    if (device.deviceID === fdevice.deviceID) {
                      device.used = true;
                    }
                  });
                });
              });

              // See if the connection is connected or undefined 
              this.systemConnectionsService.getSystemConnections().subscribe(
                connections => {
                  // TODO: check connection and request total
                  this.devices.forEach(device => {
                    // TODO make this synchronous
                    this.dbCompletionService.getDeviceCompletion(device.deviceID).subscribe(
                      c => {
                        device.completion = c.completion;
                        observer.next(device);
                      });

                    //TODO complete observer when finished
                    // observer.complete();
                  });
                }
              );

              // Synchronously get the status of each device 
              // this.getDeviceStatusInOrder(observer, 0);
            }
          )
        },
        err => { console.log("getAll error!", err) },
        () => { console.log("get all complete!") }
      );
    });
    return deviceObservable
  }
}
