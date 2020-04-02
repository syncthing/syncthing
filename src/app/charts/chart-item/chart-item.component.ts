import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-chart-item',
  templateUrl: './chart-item.component.html',
  styleUrls: ['./chart-item.component.scss']
})
export class ChartItemComponent {
  @Input() state: string;
  @Input() count: number;

  constructor() { }
}
