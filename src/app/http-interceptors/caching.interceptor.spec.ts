import { TestBed } from '@angular/core/testing';

import { CachingInterceptor } from './caching.interceptor';

describe('CachingInterceptor', () => {
  beforeEach(() => TestBed.configureTestingModule({
    providers: [
      CachingInterceptor
      ]
  }));

  it('should be created', () => {
    const interceptor: CachingInterceptor = TestBed.inject(CachingInterceptor);
    expect(interceptor).toBeTruthy();
  });
});
