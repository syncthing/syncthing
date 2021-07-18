import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { DonutChartComponent } from './donut-chart.component';
import { HttpClientModule } from '@angular/common/http';

describe('DonutChartComponent', () => {
  let component: DonutChartComponent;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [DonutChartComponent],
      providers: [DonutChartComponent]
    }).compileComponents();

    component = TestBed.inject(DonutChartComponent);
  }));

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
