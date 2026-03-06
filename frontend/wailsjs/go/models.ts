export namespace main {
	
	export class BatteryStatus {
	    percent: number;
	    charging: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BatteryStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.percent = source["percent"];
	        this.charging = source["charging"];
	    }
	}
	export class ButtonCfg {
	    Label: string;
	    Keys: string[];
	
	    static createFrom(source: any = {}) {
	        return new ButtonCfg(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Label = source["Label"];
	        this.Keys = source["Keys"];
	    }
	}
	export class RGB {
	    R: number;
	    G: number;
	    B: number;
	
	    static createFrom(source: any = {}) {
	        return new RGB(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.R = source["R"];
	        this.G = source["G"];
	        this.B = source["B"];
	    }
	}
	export class LayerDTO {
	    name: string;
	    color: RGB;
	    wheel_speed: string;
	    buttons: Record<string, ButtonCfg>;
	    // Go type: struct { Left []string "json:\"left\""; Right []string "json:\"right\"" }
	    wheel: any;
	
	    static createFrom(source: any = {}) {
	        return new LayerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = this.convertValues(source["color"], RGB);
	        this.wheel_speed = source["wheel_speed"];
	        this.buttons = this.convertValues(source["buttons"], ButtonCfg, true);
	        this.wheel = this.convertValues(source["wheel"], Object);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeviceSettingsDTO {
	    brightness: string;
	    orientation: number;
	    wheel_speed: string;
	    overlay_duration: number;
	    keyboard_layout: string;
	    sleep_timeout: number;
	    initial_layer: string;
	    double_click_ms: number;
	    button_8_double: string[];
	    show_battery_in_layer_name: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeviceSettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.brightness = source["brightness"];
	        this.orientation = source["orientation"];
	        this.wheel_speed = source["wheel_speed"];
	        this.overlay_duration = source["overlay_duration"];
	        this.keyboard_layout = source["keyboard_layout"];
	        this.sleep_timeout = source["sleep_timeout"];
	        this.initial_layer = source["initial_layer"];
	        this.double_click_ms = source["double_click_ms"];
	        this.button_8_double = source["button_8_double"];
	        this.show_battery_in_layer_name = source["show_battery_in_layer_name"];
	    }
	}
	export class ConfigDTO {
	    device: DeviceSettingsDTO;
	    layers: LayerDTO[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = this.convertValues(source["device"], DeviceSettingsDTO);
	        this.layers = this.convertValues(source["layers"], LayerDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class ValidateYAMLResult {
	    error: string;
	    line: number;
	
	    static createFrom(source: any = {}) {
	        return new ValidateYAMLResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.error = source["error"];
	        this.line = source["line"];
	    }
	}

}

