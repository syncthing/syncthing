import { Component, OnInit } from '@angular/core';
import { cardElevation } from '../../style';

@Component({
  selector: 'app-device-chart',
  templateUrl: './device-chart.component.html',
  styleUrls: ['./device-chart.component.scss']
})
export class DeviceChartComponent implements OnInit {
  chartID: string = 'devicesChart';
  elevation: string = cardElevation;

  constructor() { }

  ngOnInit(): void {
  }

}
