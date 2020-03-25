import { Injectable } from '@angular/core';
import Device from '../device';

@Injectable({
  providedIn: 'root'
})
export class DeviceService {
  private devices: Device[];
  constructor() { }
}
