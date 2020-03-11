import { Component, OnInit } from '@angular/core';

export enum ToggleAction {
  Folders = 1,
  Devices,
}

@Component({
  selector: 'app-status-toggle',
  templateUrl: './status-toggle.component.html',
  styleUrls: ['./status-toggle.component.scss']
})

export class StatusToggleComponent implements OnInit {
  public toggleAction = ToggleAction
  constructor() { }

  ngOnInit(): void {
  }

  onSelect(label: ToggleAction): void {

    console.log("here?");
    switch (label) {
      case ToggleAction.Folders:
        console.log("folder action");
        break;
      case ToggleAction.Devices:
        console.log("Device action");
        break;
    }
  }
}