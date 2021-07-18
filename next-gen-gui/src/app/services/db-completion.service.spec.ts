import { TestBed } from '@angular/core/testing';

import { DbCompletionService } from './db-completion.service';
import { HttpClientModule } from '@angular/common/http';

describe('DbCompletionService', () => {
  let service: DbCompletionService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [DbCompletionService]
    });
    service = TestBed.inject(DbCompletionService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
