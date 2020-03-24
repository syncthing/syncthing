import { TestBed } from '@angular/core/testing';

import { RequestCacheService } from './request-cache.service';

describe('RequestCacheService', () => {
  let service: RequestCacheService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(RequestCacheService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
