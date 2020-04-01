import { Component, OnInit } from '@angular/core';
import { Type } from '../../type';
import { cardElevation } from '../../style';


@Component({
  selector: 'app-status-list',
  templateUrl: './status-list.component.html',
  styleUrls: ['./status-list.component.scss']
})
export class StatusListComponent implements OnInit {
  currentListType: Type = Type.Folder;
  listType = Type; // used in html
  elevation: string = cardElevation;
  title: string = 'Status';

  constructor() { }

  ngOnInit(): void {
  }

  onToggle(t: Type) {
    this.currentListType = t;
  }
}
