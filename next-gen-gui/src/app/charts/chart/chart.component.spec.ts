import { async, TestBed } from '@angular/core/testing';

import { ChartComponent } from './chart.component';
import { HttpClientModule } from '@angular/common/http';

class MockService {
  getEach() {
    // unimplemented
  }
};

describe('ChartComponent', () => {
  let component: ChartComponent;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [ChartComponent]
    }).compileComponents();
    component = TestBed.inject(ChartComponent);
  }));

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
