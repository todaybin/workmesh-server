class Bus {
    list: { [key: string]: Array<Function> };
    constructor() {
        this.list = {};
    }

    on(name: string, fn: Function) {
        this.list[name] = this.list[name] || [];
        this.list[name].push(fn);
    }

    emit(name: string, data?: any) {
        if (this.list[name]) {
            this.list[name].forEach((fn: Function) => {
                fn(data);
            });
        }
    }

    off(name: string, fn?: Function) {
        if (!this.list[name]) return;
        if (!fn) {
            delete this.list[name];
            return;
        }
        this.list[name] = this.list[name].filter((listener) => listener !== fn);
        if (this.list[name].length === 0) delete this.list[name];
    }
}
export default Bus;
