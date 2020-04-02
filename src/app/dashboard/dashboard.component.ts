import { Component, OnInit } from '@angular/core';
import { SystemConfigService } from '../services/system-config.service';
import { StType } from '../type';
import { FilterService } from '../services/filter.service';

@Component({
  selector: 'app-dashboard',
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss'],
  providers: [FilterService]
})
export class DashboardComponent {
  folderChart: StType = StType.Folder;
  deviceChart: StType = StType.Device;

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit() {
    this.systemConfigService.getSystemConfig().subscribe(
      x => console.log('Observer got a next value: ' + x),
      err => console.error('Observer got an error: ' + err),
      () => console.log('Observer got a complete notification')
    );
  }
}
