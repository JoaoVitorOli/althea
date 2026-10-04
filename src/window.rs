use std::error::Error;

pub fn center_window_in_primary_monitor(window: &slint::Window) -> Result<(), Box<dyn Error>> {
    let Some(display) = display_info::DisplayInfo::all()?
        .into_iter()
        .find(|display| display.is_primary)
    else {
        return Ok(());
    };

    let size = window.size();

    let x = display.x + (display.width as i32 - size.width as i32) / 2;
    let y = display.y + (display.height as i32 - size.height as i32) / 2;
    window.set_position(slint::PhysicalPosition::new(x, y));

    Ok(())
}
