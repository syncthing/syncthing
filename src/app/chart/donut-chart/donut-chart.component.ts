import { Component, Input } from '@angular/core';
import { Chart } from 'chart.js'
import { SystemConfigService } from 'src/app/system-config.service';

@Component({
  selector: 'app-donut-chart',
  templateUrl: './donut-chart.component.html',
  styleUrls: ['./donut-chart.component.scss']
})
export class DonutChartComponent {
  @Input() elementID: string;
  @Input() set data(val: number[]) {
    if (this.chart) {
      val.forEach((v) => {
        this.addData(v)
      });
      console.log("set the data?", val)
    }
  };

  private canvas: any;
  private ctx: any;
  private chart: Chart;

  constructor() { }

  addData(data: number): void {
    //    this.chart.data.labels.push(label);
    this.chart.data.datasets.forEach((dataset) => {
      dataset.data.push(data);
    });
    this.chart.update();
  }


  ngAfterViewInit(): void {
    console.log("is the data set?", this.data, this.elementID);
    this.canvas = document.getElementById(this.elementID);
    this.ctx = this.canvas.getContext('2d');
    this.chart = new Chart(this.ctx, {
      type: 'doughnut',
      data: {
        labels: ["Up to Date", "Syncing", "Waiting to Sync", "Out of Sync", "Failed Items"],
        datasets: [{
          data: [],
          backgroundColor: [
            '#56C568',
            'rgba(54, 162, 235, 1)',
            'rgba(255, 206, 86, 1)'
          ],
          borderWidth: 1
        }]
      },
      options: {
        responsive: false,
        legend: {
          display: false
        }
      }
    });
  }
}
