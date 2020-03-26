import { TestBed } from '@angular/core/testing';

import { DbCompletionService } from './db-completion.service';

describe('DbCompletionService', () => {
  let service: DbCompletionService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(DbCompletionService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
