import { Component, EventEmitter, OnInit, Output, ViewChild } from '@angular/core';
import { StType } from '../type';
import { MatButtonToggleGroup } from '@angular/material/button-toggle';



@Component({
  selector: 'app-list-toggle',
  templateUrl: './list-toggle.component.html',
  styleUrls: ['./list-toggle.component.scss']
})

export class ListToggleComponent implements OnInit {
  @ViewChild(MatButtonToggleGroup) group: MatButtonToggleGroup;
  public listType = StType;
  // public toggleValue: string = "folders";
  @Output() listTypeEvent = new EventEmitter<StType>();

  constructor() { }
  ngOnInit(): void {
  }

  onSelect(t: StType): void {
    this.listTypeEvent.emit(t);
  }
}