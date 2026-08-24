const ServiceOrder = (() => {
    const categoryPriorities = {
        backend: 0,
        base: 1,
        middleware: 1,
        frontend: 2
    };

    function categoryPriority(category) {
        return Object.prototype.hasOwnProperty.call(categoryPriorities, category)
            ? categoryPriorities[category]
            : 3;
    }

    function sort(services) {
        if (!Array.isArray(services)) return [];
        return services
            .map((service, index) => ({ service, index }))
            .sort((left, right) => {
                const priorityDiff = categoryPriority(left.service?.category)
                    - categoryPriority(right.service?.category);
                return priorityDiff || left.index - right.index;
            })
            .map(item => item.service);
    }

    return { sort };
})();