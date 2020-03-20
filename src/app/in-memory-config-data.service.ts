import { Injectable } from '@angular/core';
import { config } from './mocks/mock-config'
import { dbStatus } from './mocks/mock-db-status'

@Injectable({
  providedIn: 'root'
})
export class InMemoryConfigDataService {
  createDb() {
    return { config, dbStatus };
  }

  constructor() { }
}
