// Prevent console window in addition to Slint window in Windows release builds when, e.g., starting the app via file manager. Ignored on other platforms.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::error::Error;

mod window;

slint::include_modules!();

fn main() -> Result<(), Box<dyn Error>> {
    slint::BackendSelector::new()
        .backend_name("winit".into())
        .with_winit_window_attributes_hook(|attributes| {
            use slint::winit_030::winit::platform::windows::{
                BackdropType, CornerPreference, WindowAttributesExtWindows,
            };
            use slint::winit_030::winit::window::Theme;

            attributes
                .with_theme(Some(Theme::Dark))
                .with_system_backdrop(BackdropType::TransientWindow)
                .with_corner_preference(CornerPreference::Round)
                .with_border_color(None)
        })
        .select()?;

    let ui = AppWindow::new()?;

    let ui_handle = ui.as_weak();
    ui.on_request_increase_value(move || {
        let ui = ui_handle.unwrap();
        ui.set_counter(ui.get_counter() + 1);
    });

    ui.show()?;
    window::center_window_in_primary_monitor(ui.window())?;

    ui.run()?;

    Ok(())
}
