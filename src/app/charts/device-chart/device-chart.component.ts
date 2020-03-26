import { Component, OnInit, ViewChild } from '@angular/core';
import { cardElevation } from '../../style';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';
import { DeviceService } from 'src/app/services/device.service';
import Device from '../../device';

@Component({
  selector: 'app-device-chart',
  templateUrl: './device-chart.component.html',
  styleUrls: ['./device-chart.component.scss']
})
export class DeviceChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  chartID: string = 'devicesChart';
  elevation: string = cardElevation;
  states: { label: string, count: number }[] = [];

  constructor(private deviceService: DeviceService) { }

  ngOnInit(): void { }

  ngAfterViewInit(): void {
    // TODO switch to deviceService
    this.deviceService.getAll().subscribe(
      device => {
        // Get StateType and convert to string 
        const stateType: Device.StateType = Device.getStateType(device);
        const state: string = Device.stateTypeToString(stateType);

        // Check if state exists
        let found: boolean = false;
        this.states.forEach(s => {
          if (s.label === state) {
            s.count = s.count + 1;
            found = true;
          }
        });

        if (!found) {
          this.states.push({ label: state, count: 1 });
        }

        this.donutChart.updateData(this.states);
      }
    );
  }
}
