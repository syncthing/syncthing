import { Component, OnInit, ViewChild } from '@angular/core';
import { cardElevation } from '../../style';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';
import Folder from '../../folder'
import { FolderService } from 'src/app/folder.service';

@Component({
  selector: 'app-device-chart',
  templateUrl: './device-chart.component.html',
  styleUrls: ['./device-chart.component.scss']
})
export class DeviceChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  chartID: string = 'devicesChart';
  elevation: string = cardElevation;
  states: Map<string, number>;

  constructor(private folderService: FolderService) {
    this.states = new Map();
  }

  ngOnInit(): void {

  }

  ngAfterViewInit(): void {
    // TODO switch to deviceService
    this.folderService.getAll().subscribe(
      folder => {
        // TODO: Clear existing data
        this.donutChart.data([10]);

        // Get StateType and convert to string 
        const stateType: Folder.StateType = Folder.getStateType(folder);
        const state: string = Folder.stateTypeToString(stateType);

        // Instantiate empty count states
        if (!this.states.has(state)) {
          this.states.set(state, 0);
        }
        const count: number = this.states.get(state) + 1;
        this.states.set(state, count);
      }
    );
  }
}
