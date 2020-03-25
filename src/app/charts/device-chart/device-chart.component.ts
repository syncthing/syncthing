import { Component, OnInit, ViewChild } from '@angular/core';
import { cardElevation } from '../../style';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';
import Folder from '../../folder'
import { FolderService } from 'src/app/services/folder.service';

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

  constructor(private folderService: FolderService) { }

  ngOnInit(): void {

  }

  ngAfterViewInit(): void {
    // TODO switch to deviceService
    this.folderService.getAll().subscribe(
      folder => {
        // Get StateType and convert to string 
        const stateType: Folder.StateType = Folder.getStateType(folder);
        const state: string = Folder.stateTypeToString(stateType);

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
