import { Injectable } from '@angular/core';
import { config } from './mock/mock-config'
import { dbStatus } from './mock/mock-db-status'

@Injectable({
  providedIn: 'root'
})
export class InMemoryConfigDataService {
  createDb() {
    return { config, dbStatus };
  }

  constructor() { }
}
