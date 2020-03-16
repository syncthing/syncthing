import { Component, OnInit, AfterViewInit, Input } from '@angular/core';
import { Chart } from 'chart.js'
import { flatMap } from 'rxjs/operators';

@Component({
  selector: 'app-donut-chart',
  templateUrl: './donut-chart.component.html',
  styleUrls: ['./donut-chart.component.scss']
})
export class DonutChartComponent implements OnInit {
  @Input() elementID: string;

  private canvas: any;
  private ctx: any;

  constructor() { }

  ngOnInit(): void {
  }

  ngAfterViewInit(): void {
    console.log("elementID?", this.elementID)
    this.canvas = document.getElementById(this.elementID);
    this.ctx = this.canvas.getContext('2d');
    const myChart = new Chart(this.ctx, {
      type: 'doughnut',
      data: {
        labels: ["Up to Date", "Syncing", "Waiting to Sync", "Out of Sync", "Failed Items"],
        datasets: [{
          data: [1, 2, 3, 0, 0],
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
        display: false,
        legend: {
          display: false
        }
      }
    });
  }
}
