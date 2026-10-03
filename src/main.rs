// Prevent console window in addition to Slint window in Windows release builds when, e.g., starting the app via file manager. Ignored on other platforms.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use display_info::DisplayInfo;
use std::error::Error;

slint::include_modules!();

fn main() -> Result<(), Box<dyn Error>> {
    let ui = AppWindow::new()?;

    let ui_handle = ui.as_weak();
    ui.on_request_increase_value(move || {
        let ui = ui_handle.unwrap();
        ui.set_counter(ui.get_counter() + 1);
    });

    let display = DisplayInfo::all().unwrap();

    let window_width = 800;
    let window_height = 500;

    let x = display.x + (display.width as i32 - window_width) / 2;

    let y = display.y + (display.height as i32 - window_height) / 2;

    window.set_position(slint::LogicalPosition::new(x, y));

    ui.run()?;

    Ok(())
}
