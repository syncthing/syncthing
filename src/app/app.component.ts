import { Component, OnInit } from '@angular/core';
import { SystemConfigService } from './system-config.service';

@Component({
  selector: 'app-root',
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.scss']
})
export class AppComponent {
  title = 'Tech UI';

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit(): void {
    console.log("app component init");
    this.systemConfigService.getSystemConfig().subscribe(
      x => console.log('Observer got a next value: ' + x),
      err => console.error('Observer got an error: ' + err),
      () => console.log('Observer got a complete notification')
    );
  }
}
