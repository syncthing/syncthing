import { Component, OnInit } from '@angular/core';
import { cardElevation } from '../../style';
import { SystemConfigService } from 'src/app/system-config.service';

@Component({
  selector: 'app-device-chart',
  templateUrl: './device-chart.component.html',
  styleUrls: ['./device-chart.component.scss']
})
export class DeviceChartComponent implements OnInit {
  chartID: string = 'devicesChart';
  elevation: string = cardElevation;
  data: number[];

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit(): void {
    this.systemConfigService.getFolders().subscribe(
      data => {
        this.data = [0, 230, 32, 40];
      }
    );
  }
}
