import { TestBed } from '@angular/core/testing';

import { DbStatusService } from './db-status.service';
import { HttpClientModule } from '@angular/common/http';

describe('DbStatusService', () => {
  let service: DbStatusService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [DbStatusService]
    });
    TestBed.configureTestingModule({});
    service = TestBed.inject(DbStatusService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
