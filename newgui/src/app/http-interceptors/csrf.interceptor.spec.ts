import { TestBed } from '@angular/core/testing';

import { CSRFInterceptor } from './csrf.interceptor';

describe('CsrfInterceptor', () => {
  beforeEach(() => TestBed.configureTestingModule({
    providers: [
      CSRFInterceptor
    ]
  }));

  it('should be created', () => {
    const interceptor: CSRFInterceptor = TestBed.inject(CSRFInterceptor);
    expect(interceptor).toBeTruthy();
  });
});
