import { Component, Input } from '@angular/core';
import { Chart } from 'chart.js'
import { tooltip } from '../tooltip'

@Component({
  selector: 'app-donut-chart',
  templateUrl: './donut-chart.component.html',
  styleUrls: ['./donut-chart.component.scss']
})
export class DonutChartComponent {
  @Input() elementID: string;

  private canvas: any;
  private ctx: any;
  private chart: Chart;

  constructor() { }

  data(val: number[]) {
    if (this.chart) {
      val.forEach((v) => {
        this.addData(v)
      });
    }
  }

  updateData(data: { label: string, count: number }[]): void {
    //Using object destructuring
    for (let i = 0; i < data.length; i++) {
      let s = data[i];
      this.chart.data.labels[i] = s.label;
      this.chart.data.datasets[0].data[i] = s.count;
    }
    this.chart.update();
  }

  addData(data: number): void {
    //    this.chart.data.labels.push(label);
    this.chart.data.datasets.forEach((dataset) => {
      dataset.data.push(data);
    });
    this.chart.update();
  }

  removeAllData(withAnimation: boolean): void {
    this.chart.data.labels.pop();
    this.chart.data.datasets.forEach((dataset) => {
      dataset.data = [];
    });
    this.chart.update(withAnimation);
  }

  ngAfterViewInit(): void {
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
        },
        tooltips: {
          // Disable the on-canvas tooltip
          enabled: false,
          custom: tooltip(),
        }
      }
    });
  }
}
