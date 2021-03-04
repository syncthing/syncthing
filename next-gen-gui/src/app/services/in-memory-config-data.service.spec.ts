import { TestBed } from '@angular/core/testing';

import { InMemoryConfigDataService } from './in-memory-config-data.service';

describe('InMemoryDataService', () => {
  let service: InMemoryConfigDataService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(InMemoryConfigDataService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
