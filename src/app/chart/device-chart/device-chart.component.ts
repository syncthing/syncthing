import { Component, OnInit, ViewChild } from '@angular/core';
import { cardElevation } from '../../style';
import { SystemConfigService } from 'src/app/system-config.service';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';

@Component({
  selector: 'app-device-chart',
  templateUrl: './device-chart.component.html',
  styleUrls: ['./device-chart.component.scss']
})
export class DeviceChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;

  chartID: string = 'devicesChart';
  elevation: string = cardElevation;

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit(): void {

  }

  ngAfterViewInit(): void {
    this.systemConfigService.getDevices().subscribe(
      devices => {
        this.donutChart.data([0, 230, 32, 40]);
      }
    );
  }
}
