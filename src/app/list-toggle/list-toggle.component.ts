import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { StType } from '../type';



@Component({
  selector: 'app-list-toggle',
  templateUrl: './list-toggle.component.html',
  styleUrls: ['./list-toggle.component.scss']
})

export class ListToggleComponent implements OnInit {
  public listType = StType;
  public toggleValue: string = "folders";
  @Output() listTypeEvent = new EventEmitter<StType>();

  constructor() { }
  ngOnInit(): void {
  }

  onSelect(t: StType): void {
    this.listTypeEvent.emit(t);
  }
}