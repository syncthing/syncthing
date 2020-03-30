import { Component, OnInit } from '@angular/core';
import { SystemConfigService } from '../services/system-config.service';
import { Type } from '../type';

@Component({
  selector: 'app-dashboard',
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss']
})
export class DashboardComponent {
  folderChart: Type = Type.Folder;
  deviceChart: Type = Type.Device;

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit() {
    this.systemConfigService.getSystemConfig().subscribe(
      x => console.log('Observer got a next value: ' + x),
      err => console.error('Observer got an error: ' + err),
      () => console.log('Observer got a complete notification')
    );
  }
}
