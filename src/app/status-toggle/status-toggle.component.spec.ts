import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { StatusToggleComponent } from './status-toggle.component';

describe('StatusToggleComponent', () => {
  let component: StatusToggleComponent;
  let fixture: ComponentFixture<StatusToggleComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ StatusToggleComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(StatusToggleComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
