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

  constructor() { }

  private filterChangeSource = new BehaviorSubject<FilterInput>({ type: StType.Folder, text: "" });

  filterChanged$ = this.filterChangeSource.asObservable();

  changeFilter(input: FilterInput) {
    this.filterChangeSource.next(input);
  }
}
