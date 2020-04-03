import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-chart-item',
  templateUrl: './chart-item.component.html',
  styleUrls: ['./chart-item.component.scss']
})
export class ChartItemComponent {
  @Input() state: string;
  @Input() count: number;
  @Input('selected')
  set selected(s: boolean) {
    this._selected = s;
  }

  _selected: boolean = true;

  constructor() { }
}
