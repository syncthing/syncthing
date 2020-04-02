import { Injectable } from '@angular/core';
import { StType } from '../type';
import { BehaviorSubject } from 'rxjs';

@Injectable({
  providedIn: 'root'
})

export interface FilterInput {
  type: StType;
  text: string
}

export class FilterService {
  previousInputs = new Map<StType, string>(
    [
      [StType.Folder, ""],
      [StType.Device, ""],
    ]

  )

  constructor() { }

  private filterChangeSource = new BehaviorSubject<FilterInput>({ type: StType.Folder, text: "" });

  filterChanged$ = this.filterChangeSource.asObservable();

  changeFilter(input: FilterInput) {
    this.previousInputs.set(input.type, input.text)
    this.filterChangeSource.next(input);
  }
}
