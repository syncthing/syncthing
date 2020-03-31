import { Component, OnInit, ViewChild, Input } from '@angular/core';
import Folder from '../../folder'
import { cardElevation } from '../../style'
import { FolderService } from 'src/app/services/folder.service';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';
import { DeviceService } from 'src/app/services/device.service';
import Device from 'src/app/device';
import { Type } from '../../type';



@Component({
  selector: 'app-chart',
  templateUrl: './chart.component.html',
  styleUrls: ['./chart.component.scss']
})

export class ChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  @Input() type: Type;
  title: string;
  chartID: string;
  states: { label: string, count: number, color: string }[] = [];
  elevation: string = cardElevation;
  service: any;
  namespace: any;

  constructor(private folderService: FolderService, private deviceServce: DeviceService) { }

  ngOnInit(): void {
    switch (this.type) {
      case Type.Folder:
        this.title = "Folders";
        this.chartID = 'foldersChart';
        this.service = this.folderService;
        break;
      case Type.Device:
        this.title = "Devices";
        this.chartID = 'devicesChart';
        this.service = this.deviceServce;
        break;
    }
  }

  ngAfterViewInit() {
    let totalCount: number = 0;
    this.service.getAll().subscribe(
      t => {
        // Count the number of folders and set chart
        totalCount++;
        this.donutChart.count = totalCount;

        // Get StateType and convert to string 
        const stateType = t.stateType;
        const state = t.state;
        let color;
        switch (this.type) {
          case Type.Folder:
            color = Folder.stateTypeToColor(t.stateType);
            break;
          case Type.Device:
            color = Device.stateTypeToColor(stateType);
            break;
        }

        // Check if state exists
        let found: boolean = false;
        this.states.forEach(s => {
          if (s.label === state) {
            s.count = s.count + 1;
            found = true;
          }
        });

        if (!found) {
          console.log(color, "look!!!")
          this.states.push({ label: state, count: 1, color: color });
        }

        this.donutChart.updateData(this.states);
      },
      err => console.error('Observer got an error: ' + err),
      () => {
      }
    );
  }
}