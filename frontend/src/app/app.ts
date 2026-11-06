import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { suppressDefaultContextMenu } from '@core/context-menu';
import { ThemeStore } from '@core/theme-store';

@Component({
  imports: [RouterOutlet],
  selector: 'app-root',
  styleUrl: './app.css',
  templateUrl: './app.html',
})
export class App {
  // Created at startup so the saved theme applies before anything renders.
  private readonly theme = inject(ThemeStore);

  constructor() {
    suppressDefaultContextMenu();
  }
}
