import { Component, OnInit, Input } from '@angular/core';

@Component({
  selector: 'app-chart-item',
  templateUrl: './chart-item.component.html',
  styleUrls: ['./chart-item.component.scss']
})
export class ChartItemComponent implements OnInit {
  @Input() state: string;
  @Input() count: number;
  constructor() { }

  ngOnInit(): void {
  }

}
