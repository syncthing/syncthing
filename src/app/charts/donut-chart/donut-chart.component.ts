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
  @Input() title: number;
  _count: number;
  _countClass = "count-total";
  set count(n: number) {
    if (n >= 1000) { // use a smaller font
      this._countClass = "large-count-total"
    }
    this._count = n;
  }

  private canvas: any;
  private ctx: any;
  private chart: Chart;

  constructor() { }

  updateData(data: { label: string, count: number, color: string }[]): void {
    // Using object destructuring
    for (let i = 0; i < data.length; i++) {
      let s = data[i];
      this.chart.data.labels[i] = s.label;
      this.chart.data.datasets[0].data[i] = s.count;
      this.chart.data.datasets[0].backgroundColor[i] = s.color;
    }
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
        datasets: [{
          data: [],
          backgroundColor: [],
          borderWidth: 1
        }]
      },
      options: {
        cutoutPercentage: 77,
        responsive: true,
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