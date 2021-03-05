import { Component } from '@angular/core';
import { SystemConfigService } from './services/system-config.service';
import { MessageService } from './services/message.service';
import { catchError } from 'rxjs/operators';

@Component({
  selector: 'app-root',
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.scss']
})
export class AppComponent {
  constructor(
    private systemConfigService: SystemConfigService,
    private messageService: MessageService,
  ) { }

  restoreDefaultTheme() {
    this.systemConfigService.setGUITheme('default')
    .subscribe(() => {
      this.messageService.add('The default GUI theme has been selected. Please hit "Reload" in your browser.')
    })
  }
}
