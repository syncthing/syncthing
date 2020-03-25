import { TestBed } from '@angular/core/testing';

import { DbStatusService } from './db-status.service';

describe('DbStatusService', () => {
  let service: DbStatusService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(DbStatusService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
