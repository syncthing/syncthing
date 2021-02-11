import { Component, OnInit } from '@angular/core';
import { cardElevation } from '../style';

@Component({
  selector: 'app-card',
  template: '<div class="{{elevation}} tui-card"><ng-content></ng-content></div>',
  styleUrls: ['./card.component.scss']
})
export class CardComponent implements OnInit {
  elevation: string = cardElevation;

  constructor() { }

  ngOnInit(): void {
  }

}

@Component({
  selector: 'app-card-title',
  template: '<div class="tui-card-title"><ng-content></ng-content></div>',
  styleUrls: ['./card.component.scss']
})
export class CardTitleComponent {
  constructor() { }
}

@Component({
  selector: 'app-card-content',
  template: '<div class="tui-card-content"><ng-content></ng-content></div>',
  styleUrls: ['./card.component.scss']
})
export class CardContentComponent {
  constructor() { }
}
