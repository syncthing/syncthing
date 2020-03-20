import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { DeviceChartComponent } from './device-chart.component';

describe('DeviceChartComponent', () => {
  let component: DeviceChartComponent;
  let fixture: ComponentFixture<DeviceChartComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ DeviceChartComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(DeviceChartComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
