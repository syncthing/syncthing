import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { ListToggleComponent } from './list-toggle.component';

describe('ListToggleComponent', () => {
  let component: ListToggleComponent;
  let fixture: ComponentFixture<ListToggleComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ListToggleComponent]
    })
      .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ListToggleComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
