import { Component, OnInit } from '@angular/core';
import { ListType } from '../../list-type';
import { cardElevation } from '../../style';


@Component({
  selector: 'app-status-list',
  templateUrl: './status-list.component.html',
  styleUrls: ['./status-list.component.scss']
})
export class StatusListComponent implements OnInit {
  currentListType: ListType = ListType.Folders;
  listType = ListType; // used in html
  elevation: string = cardElevation;
  title: string = 'Status';

  constructor() { }

  ngOnInit(): void {
  }

  onToggle(t: ListType) {
    this.currentListType = t;
  }
}
