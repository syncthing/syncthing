import { Injectable } from '@angular/core';
import { config } from '../mocks/mock-config'
import { dbStatus } from '../mocks/mock-db-status'
import { connections } from '../mocks/mock-connections'
import { dbCompletion } from '../mocks/mock-db-completion'

@Injectable({
  providedIn: 'root'
})
export class InMemoryConfigDataService {
  createDb() {
    return { config, dbStatus, connections, dbCompletion };
  }

  constructor() { }
}
